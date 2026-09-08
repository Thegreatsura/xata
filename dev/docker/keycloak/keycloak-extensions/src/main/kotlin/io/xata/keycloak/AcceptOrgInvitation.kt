package io.xata.keycloak

import org.keycloak.Config
import org.keycloak.authentication.AuthenticationFlowContext
import org.keycloak.authentication.Authenticator
import org.keycloak.authentication.AuthenticatorFactory
import org.keycloak.authentication.authenticators.broker.AbstractIdpAuthenticator
import org.keycloak.authentication.authenticators.broker.util.SerializedBrokeredIdentityContext
import org.keycloak.broker.provider.BrokeredIdentityContext
import org.keycloak.events.Details
import org.keycloak.events.EventType
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.models.RealmModel
import org.keycloak.models.UserModel
import org.keycloak.organization.OrganizationProvider
import org.keycloak.provider.ProviderConfigProperty

/**
 * Adds an invitee to the organization that invited them, whichever identity provider they used.
 * Belongs in first broker login, after the account has been created or linked.
 *
 * Stock Keycloak only adds a member when the provider belongs to the organization, which a social
 * login shared by every organization never does, so an invitation accepted that way is dropped.
 *
 * See [OrgInvitation].
 */
class AcceptOrgInvitation : AbstractIdpAuthenticator() {
    override fun authenticateImpl(
        context: AuthenticationFlowContext,
        serializedCtx: SerializedBrokeredIdentityContext,
        brokerContext: BrokeredIdentityContext,
    ) {
        val user = context.user
        val pending = OrgInvitation.pending(context.session, context.authenticationSession, user?.email)

        if (pending == null) {
            // Nothing to accept. Leave the rest of the flow alone.
            context.success()
            return
        }

        // The invitation names this user for this organization, so the provider does not matter.
        // Unmanaged, matching how an existing user accepts one through the action token.
        val provider = context.session.getProvider(OrganizationProvider::class.java)
        provider.addMember(pending.organization, user)
        provider.invitationManager.remove(pending.token.id)
        pending.token.redirectUri?.let { context.authenticationSession.redirectUri = it }

        context.event.clone()
            .event(EventType.INVITE_ORG)
            .user(user)
            .detail(Details.USERNAME, user.username)
            .detail(Details.ORG_ID, pending.organization.id)
            .success()

        context.success()
    }

    override fun actionImpl(
        context: AuthenticationFlowContext,
        serializedCtx: SerializedBrokeredIdentityContext,
        brokerContext: BrokeredIdentityContext,
    ) = Unit

    override fun requiresUser(): Boolean = true

    // Must be true: a REQUIRED execution whose authenticator is not "configured for" the user
    // aborts the flow with "credential setup required".
    override fun configuredFor(
        session: KeycloakSession,
        realm: RealmModel,
        user: UserModel,
    ): Boolean = true
}

class AcceptOrgInvitationFactory : AuthenticatorFactory {
    companion object {
        const val PROVIDER_ID = "xata-accept-org-invitation"
        private val SINGLETON = AcceptOrgInvitation()
    }

    override fun create(session: KeycloakSession?): Authenticator = SINGLETON

    override fun init(config: Config.Scope?) = Unit

    override fun postInit(factory: KeycloakSessionFactory?) = Unit

    override fun close() = Unit

    override fun getId(): String = PROVIDER_ID

    override fun getDisplayType(): String = "Accept Organization Invitation"

    override fun getHelpText(): String =
        "Adds an invited user to the organization that invited them, whichever identity provider " +
            "they signed in with."

    override fun getReferenceCategory(): String = "organization"

    override fun isConfigurable(): Boolean = false

    override fun getConfigProperties(): MutableList<ProviderConfigProperty> = mutableListOf()

    override fun getRequirementChoices(): Array<AuthenticationExecutionModel.Requirement> =
        arrayOf(
            AuthenticationExecutionModel.Requirement.REQUIRED,
            AuthenticationExecutionModel.Requirement.DISABLED,
        )

    override fun isUserSetupAllowed(): Boolean = false
}
