package io.xata.keycloak

import org.keycloak.TokenVerifier
import org.keycloak.authentication.actiontoken.inviteorg.InviteOrgActionToken
import org.keycloak.crypto.CryptoUtils
import org.keycloak.http.HttpRequest
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationModel
import org.keycloak.organization.OrganizationProvider
import org.keycloak.services.Urls
import org.keycloak.sessions.AuthenticationSessionModel

/**
 * Carries an organization invitation across the redirect to an identity provider.
 *
 * Keycloak resolves the invitation onto the request context and nowhere else, so it is gone by the
 * time the invitee returns from the provider: the broker login URL does not carry the token, and
 * [org.keycloak.authentication.AuthenticationProcessor.resetFlow] clears every auth note before
 * first broker login runs. It leaves client notes alone, which is what this uses.
 *
 * Delete this and both authenticators once a Keycloak carrying keycloak/keycloak#52183 is released.
 */
object OrgInvitation {
    const val TOKEN_NOTE = "xata.org.invitation.token"

    data class Pending(val token: InviteOrgActionToken, val organization: OrganizationModel)

    /** The invitation token on the request, or null if there is none or it does not verify. */
    fun tokenOnRequest(
        session: KeycloakSession,
        request: HttpRequest,
    ): String? = request.uri.queryParameters.getFirst("token")?.takeIf { verify(session, it) != null }

    /** The invitation this login is accepting, or null if there is none or it does not apply to [email]. */
    fun pending(
        session: KeycloakSession,
        authSession: AuthenticationSessionModel?,
        email: String?,
    ): Pending? {
        if (authSession == null || email == null) return null

        val token = verify(session, authSession.getClientNote(TOKEN_NOTE)) ?: return null
        if (!email.equals(token.email, ignoreCase = true)) return null

        val provider = session.getProvider(OrganizationProvider::class.java)
        val organization = provider.getById(token.orgId)?.takeIf { it.isEnabled } ?: return null
        val invitation = provider.invitationManager.getById(token.id)

        return if (invitation == null || invitation.isExpired) null else Pending(token, organization)
    }

    /** Mirrors Organizations.parseInvitationToken, which has no String overload before #52183. */
    private fun verify(
        session: KeycloakSession,
        tokenString: String?,
    ): InviteOrgActionToken? {
        if (tokenString == null) return null

        return runCatching {
            val context = session.context
            val verifier =
                TokenVerifier.create(tokenString, InviteOrgActionToken::class.java)
                    .withChecks(
                        TokenVerifier.IS_ACTIVE,
                        TokenVerifier.RealmUrlCheck(Urls.realmIssuer(context.uri.baseUri, context.realm.name)),
                    )
            verifier.verifierContext(
                CryptoUtils.getSignatureProvider(session, verifier.header.algorithm.name)
                    .verifier(verifier.header.keyId),
            )
            verifier.verify().token
        }.getOrNull()
    }
}
