package io.xata.keycloak

import org.keycloak.TokenVerifier
import org.keycloak.authentication.actiontoken.inviteorg.InviteOrgActionToken
import org.keycloak.broker.provider.AbstractIdentityProvider
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

        // The address came from the review profile form rather than from the provider, so nothing
        // vouches for it yet and a verification mail is still pending. Anyone who saw the invitation
        // link could otherwise type the invited address and join on the spot. Keycloak reads the same
        // note before it lets a brokered login skip e-mail verification.
        if (authSession.getAuthNote(AbstractIdentityProvider.UPDATE_PROFILE_EMAIL_CHANGED).toBoolean()) return null

        val token = verify(session, authSession.getClientNote(TOKEN_NOTE)) ?: return null
        if (!email.equals(token.email, ignoreCase = true)) return null

        val provider = session.getProvider(OrganizationProvider::class.java)
        val organization = provider.getById(token.orgId)?.takeIf { it.isEnabled } ?: return null
        val invitation = provider.invitationManager.getById(token.id)

        return if (invitation == null || invitation.isExpired) null else Pending(token, organization)
    }

    /**
     * Mirrors Organizations.parseInvitationToken, which has no String overload before #52183.
     *
     * The action token endpoint picks a handler by token type and so never mixes two kinds of token
     * up; reading the token by hand skips that, hence the explicit type check. Single use is not
     * enforced here, unlike on that endpoint: the same string is verified once when it is captured
     * and again when the invitation is accepted. Acceptance is what consumes it, by removing the
     * invitation the token names.
     */
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
                    ).tokenType(listOf(InviteOrgActionToken.TOKEN_TYPE))
            verifier.verifierContext(
                CryptoUtils.getSignatureProvider(session, verifier.header.algorithm.name)
                    .verifier(verifier.header.keyId),
            )
            verifier.verify().token
        }.getOrNull()
    }
}
