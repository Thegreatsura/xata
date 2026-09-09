package io.xata.keycloak

import jakarta.ws.rs.core.Response
import org.jboss.logging.Logger
import org.keycloak.Config
import org.keycloak.authentication.AuthenticationFlowContext
import org.keycloak.authentication.AuthenticationFlowError
import org.keycloak.authentication.AuthenticationFlowException
import org.keycloak.authentication.Authenticator
import org.keycloak.authentication.AuthenticatorFactory
import org.keycloak.authentication.FlowStatus
import org.keycloak.authentication.authenticators.broker.AbstractIdpAuthenticator
import org.keycloak.authentication.authenticators.broker.util.PostBrokerLoginConstants
import org.keycloak.authentication.authenticators.broker.util.SerializedBrokeredIdentityContext
import org.keycloak.authentication.authenticators.browser.IdentityProviderAuthenticator
import org.keycloak.events.Errors
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.organization.utils.Organizations
import org.keycloak.provider.ProviderConfigProperty
import org.keycloak.sessions.AuthenticationSessionModel

/**
 * Holds a brokered login to the provider its email domain belongs to.
 *
 * An organization's "Redirect when email domain matches" only fires on the identity-first login
 * page, where an email has been typed. A provider button links straight to
 * `/broker/{alias}/login`, so the organization step never runs and no domain is ever matched: an
 * `@acme.com` user can sign in through the shared Google or GitHub button and end up with an
 * account that never passed through their organization's provider.
 *
 * Belongs on the shared providers, in both broker flows: as the first login flow override, where it
 * runs before an account is created; and as the post login flow, the only hook a returning user
 * with an existing link still goes through.
 *
 * See [DomainSso].
 */
class RequireDomainSso : IdentityProviderAuthenticator() {
    companion object {
        private val LOGGER: Logger = Logger.getLogger(RequireDomainSso::class.java)
    }

    override fun authenticate(context: AuthenticationFlowContext) {
        val authSession = context.authenticationSession
        val brokered =
            brokeredContext(authSession) ?: throw AuthenticationFlowException(
                "No brokered context in the authentication session. " +
                    "${RequireDomainSsoFactory.PROVIDER_ID} only belongs in a broker flow.",
                AuthenticationFlowError.INTERNAL_ERROR,
            )

        val email = brokered.email ?: context.user?.email
        val required = DomainSso.requiredBroker(context.session, Organizations.getEmailDomain(email))

        if (required == null || required.alias == brokered.identityProviderId) {
            context.success()
            return
        }

        LOGGER.infof(
            "Domain of '%s' belongs to identity provider '%s', not '%s'",
            email,
            required.alias,
            brokered.identityProviderId,
        )

        if (!deniesOnMismatch(context)) {
            // IdentityBrokerService reads a leftover brokered context as a nested first broker
            // login and bounces straight back to this execution, so drop both before handing over.
            authSession.setAuthNote(AbstractIdpAuthenticator.BROKERED_CONTEXT_NOTE, null)
            authSession.setAuthNote(PostBrokerLoginConstants.PBL_BROKERED_IDENTITY_CONTEXT, null)

            redirect(context, required.alias, email)
            if (context.status == FlowStatus.FORCE_CHALLENGE) return

            // redirect() gives up quietly when the provider has gone; refusing beats letting the
            // login it was placed here to stop through.
            LOGGER.warnf("Could not redirect to identity provider '%s'", required.alias)
        }

        deny(context, required)
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

    private fun deny(
        context: AuthenticationFlowContext,
        required: IdentityProviderModel,
    ) {
        context.event.user(context.user).error(Errors.NOT_ALLOWED)

        val name = required.displayName ?: required.alias
        val page =
            context.form()
                .setError("Your organization signs in through $name. Use that to continue.")
                .createErrorPage(Response.Status.FORBIDDEN)

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
        "Stops a login through a shared identity provider when the email domain belongs to an " +
            "organization that redirects it somewhere else."

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
