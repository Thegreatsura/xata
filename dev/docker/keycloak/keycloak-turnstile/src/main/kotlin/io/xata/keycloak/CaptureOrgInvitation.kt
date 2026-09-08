package io.xata.keycloak

import org.keycloak.Config
import org.keycloak.authentication.AuthenticationFlowContext
import org.keycloak.authentication.Authenticator
import org.keycloak.authentication.AuthenticatorFactory
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.models.RealmModel
import org.keycloak.models.UserModel
import org.keycloak.provider.ProviderConfigProperty

/**
 * Pins the invitation token to the authentication session. Belongs at the top of the registration
 * flow, where the invitation link lands and the token is still on the request.
 *
 * See [OrgInvitation].
 */
class CaptureOrgInvitation : Authenticator {
    override fun authenticate(context: AuthenticationFlowContext) {
        OrgInvitation.tokenOnRequest(context.session, context.httpRequest)?.let {
            context.authenticationSession.setClientNote(OrgInvitation.TOKEN_NOTE, it)
        }
        context.success()
    }

    override fun action(context: AuthenticationFlowContext) = context.success()

    override fun requiresUser(): Boolean = false

    override fun configuredFor(
        session: KeycloakSession,
        realm: RealmModel,
        user: UserModel,
    ): Boolean = true

    override fun setRequiredActions(
        session: KeycloakSession,
        realm: RealmModel,
        user: UserModel,
    ) = Unit

    override fun close() = Unit
}

class CaptureOrgInvitationFactory : AuthenticatorFactory {
    companion object {
        const val PROVIDER_ID = "xata-capture-org-invitation"
        private val SINGLETON = CaptureOrgInvitation()
    }

    override fun create(session: KeycloakSession?): Authenticator = SINGLETON

    override fun init(config: Config.Scope?) = Unit

    override fun postInit(factory: KeycloakSessionFactory?) = Unit

    override fun close() = Unit

    override fun getId(): String = PROVIDER_ID

    override fun getDisplayType(): String = "Hold On To Organization Invitation"

    override fun getHelpText(): String =
        "Remembers the organization invitation that opened this page, so it still applies after the " +
            "invitee signs in with an identity provider."

    override fun getReferenceCategory(): String? = null

    override fun isConfigurable(): Boolean = false

    override fun getConfigProperties(): MutableList<ProviderConfigProperty> = mutableListOf()

    override fun getRequirementChoices(): Array<AuthenticationExecutionModel.Requirement> =
        arrayOf(
            AuthenticationExecutionModel.Requirement.REQUIRED,
            AuthenticationExecutionModel.Requirement.DISABLED,
        )

    override fun isUserSetupAllowed(): Boolean = false
}
