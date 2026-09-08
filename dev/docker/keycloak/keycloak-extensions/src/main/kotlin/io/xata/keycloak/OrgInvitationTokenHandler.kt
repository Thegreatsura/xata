package io.xata.keycloak

import jakarta.ws.rs.core.Response
import org.keycloak.authentication.actiontoken.ActionTokenContext
import org.keycloak.authentication.actiontoken.inviteorg.InviteOrgActionToken
import org.keycloak.authentication.actiontoken.inviteorg.InviteOrgActionTokenHandler
import org.keycloak.organization.OrganizationProvider
import org.keycloak.organization.utils.Organizations

/**
 * Puts the inviting organization on the request context so the login forms can name it.
 *
 * Keycloak only does that from [InviteOrgActionTokenHandler.preHandleToken], which runs on the
 * registration entry points alone. An invitee who already has an account reaches `handleToken`
 * directly, where `Organizations.resolveOrganization` falls through to a membership check they
 * cannot pass yet, so the confirm-membership page is left with the bare organization id.
 *
 * Temporary, until a Keycloak carrying keycloak/keycloak#52397 is released.
 */
class OrgInvitationTokenHandler : InviteOrgActionTokenHandler() {
    override fun handleToken(
        token: InviteOrgActionToken,
        tokenContext: ActionTokenContext<InviteOrgActionToken>,
    ): Response {
        val session = tokenContext.session

        if (Organizations.isEnabled(session)) {
            session.getProvider(OrganizationProvider::class.java)
                .getById(token.orgId)
                ?.takeIf { it.isEnabled }
                ?.let { session.context.organization = it }
        }

        return super.handleToken(token, tokenContext)
    }

    // Overrides the built-in handler, which registers the same ORGIVT id.
    override fun order(): Int = 100
}
