package io.xata.keycloak

import jakarta.ws.rs.core.Response
import org.jboss.logging.Logger
import org.keycloak.Config
import org.keycloak.authentication.AuthenticationFlowContext
import org.keycloak.authentication.AuthenticationFlowError
import org.keycloak.authentication.Authenticator
import org.keycloak.authentication.AuthenticatorFactory
import org.keycloak.authentication.FlowStatus
import org.keycloak.authentication.authenticators.broker.AbstractIdpAuthenticator
import org.keycloak.authentication.authenticators.broker.util.PostBrokerLoginConstants
import org.keycloak.authentication.authenticators.broker.util.SerializedBrokeredIdentityContext
import org.keycloak.authentication.authenticators.browser.AbstractUsernameFormAuthenticator
import org.keycloak.authentication.authenticators.browser.IdentityProviderAuthenticator
import org.keycloak.events.Errors
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.provider.ProviderConfigProperty
import org.keycloak.sessions.AuthenticationSessionModel

/**
 * Holds a login to the provider its email domain belongs to.
 *
 * Keycloak's own redirect only fires on the identity-first page and only for a user with no
 * first-factor credential, so a provider button, a password, a reset and a registration all reach
 * the realm without a domain ever being matched. Belongs in first broker login, post broker login,
 * browser forms and reset credentials; registration is a form flow and takes
 * [RegistrationRequireDomainSso].
 *
 * See [DomainSso].
 */
class RequireDomainSso : IdentityProviderAuthenticator() {
    companion object {
        private val LOGGER: Logger = Logger.getLogger(RequireDomainSso::class.java)
    }

    override fun authenticate(context: AuthenticationFlowContext) {
        val authSession = context.authenticationSession
        val brokered = brokeredContext(authSession)

        // A broker flow always carries an email; the browser and reset flows only after a step
        // that collects a username. With none of them this is a no-op.
        val email =
            brokered?.email
                ?: context.user?.email
                ?: authSession.getAuthNote(AbstractUsernameFormAuthenticator.ATTEMPTED_USERNAME)

        if (brokered != null && !DomainSso.assertsOwnDomain(context.session, brokered.identityProviderId, email)) {
            LOGGER.warnf(
                "Identity provider '%s' asserted '%s', which is outside the domain it is bound to",
                brokered.identityProviderId,
                email,
            )
            refuse(
                context,
                "That identity provider is not allowed to sign in this email address.",
            )
            return
        }

        val required = DomainSso.mismatchedBroker(context.session, email, brokered?.identityProviderId)

        if (required == null) {
            context.success()
            return
        }

        LOGGER.infof(
            "Domain of '%s' belongs to identity provider '%s', not '%s'",
            email,
            required.alias,
            brokered?.identityProviderId ?: "a local credential",
        )

        if (!deniesOnMismatch(context)) {
            // IdentityBrokerService reads a leftover brokered context as a nested first broker
            // login and bounces straight back here, so drop both before handing over.
            authSession.setAuthNote(AbstractIdpAuthenticator.BROKERED_CONTEXT_NOTE, null)
            authSession.setAuthNote(PostBrokerLoginConstants.PBL_BROKERED_IDENTITY_CONTEXT, null)

            redirect(context, required.alias, email)
            if (context.status == FlowStatus.FORCE_CHALLENGE) return

            // redirect() gives up quietly when the provider has gone, so refuse instead.
            LOGGER.warnf("Could not redirect to identity provider '%s'", required.alias)
        }

        refuse(context, DomainSso.useYourProvider(required, "Use that to continue."))
    }

    /** The identity just brokered, from whichever of the two broker flows this is running in. */
    private fun brokeredContext(authSession: AuthenticationSessionModel): SerializedBrokeredIdentityContext? =
        SerializedBrokeredIdentityContext.readFromAuthenticationSession(
            authSession,
            AbstractIdpAuthenticator.BROKERED_CONTEXT_NOTE,
        ) ?: SerializedBrokeredIdentityContext.readFromAuthenticationSession(
            authSession,
            PostBrokerLoginConstants.PBL_BROKERED_IDENTITY_CONTEXT,
        )

    private fun deniesOnMismatch(context: AuthenticationFlowContext): Boolean =
        context.authenticatorConfig?.config?.get(RequireDomainSsoFactory.ON_MISMATCH) ==
            RequireDomainSsoFactory.DENY

    private fun refuse(
        context: AuthenticationFlowContext,
        message: String,
    ) {
        context.event.user(context.user).error(Errors.NOT_ALLOWED)
        val page = context.form().setError(message).createErrorPage(Response.Status.FORBIDDEN)
        context.failureChallenge(AuthenticationFlowError.ACCESS_DENIED, page)
    }
}

class RequireDomainSsoFactory : AuthenticatorFactory {
    companion object {
        const val PROVIDER_ID = "xata-require-domain-sso"
        const val ON_MISMATCH = "on-mismatch"
        const val REDIRECT = "redirect"
        const val DENY = "deny"
        private val SINGLETON = RequireDomainSso()
    }

    override fun create(session: KeycloakSession?): Authenticator = SINGLETON

    override fun init(config: Config.Scope?) = Unit

    override fun postInit(factory: KeycloakSessionFactory?) = Unit

    override fun close() = Unit

    override fun getId(): String = PROVIDER_ID

    override fun getDisplayType(): String = "Require Organization SSO For Email Domain"

    override fun getHelpText(): String =
        "Stops a login when the email domain belongs to an organization that redirects it to its " +
            "own identity provider. Place it in both broker flows, in the browser forms flow after " +
            "the organization step, and in the reset credentials flow after the user is resolved."

    override fun getReferenceCategory(): String = "organization"

    override fun isConfigurable(): Boolean = true

    override fun getConfigProperties(): MutableList<ProviderConfigProperty> =
        mutableListOf(
            ProviderConfigProperty().apply {
                name = ON_MISMATCH
                label = "On mismatch"
                helpText =
                    "Send the user on to the provider their organization requires, or refuse the " +
                    "login outright."
                type = ProviderConfigProperty.LIST_TYPE
                options = listOf(REDIRECT, DENY)
                defaultValue = REDIRECT
            },
        )

    override fun getRequirementChoices(): Array<AuthenticationExecutionModel.Requirement> =
        arrayOf(
            AuthenticationExecutionModel.Requirement.REQUIRED,
            AuthenticationExecutionModel.Requirement.DISABLED,
        )

    override fun isUserSetupAllowed(): Boolean = false
}
