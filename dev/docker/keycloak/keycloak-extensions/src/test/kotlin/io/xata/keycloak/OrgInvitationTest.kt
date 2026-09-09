package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Test
import org.keycloak.broker.provider.AbstractIdentityProvider
import org.keycloak.models.KeycloakSession
import org.keycloak.sessions.AuthenticationSessionModel
import kotlin.test.assertNull

class OrgInvitationTest {
    private val session = mockk<KeycloakSession>()

    private fun authSession(emailChanged: String?): AuthenticationSessionModel =
        mockk<AuthenticationSessionModel> {
            every { getAuthNote(AbstractIdentityProvider.UPDATE_PROFILE_EMAIL_CHANGED) } returns emailChanged
            every { getClientNote(OrgInvitation.TOKEN_NOTE) } returns null
        }

    @Test
    fun `pending is null without an authentication session`() {
        assertNull(OrgInvitation.pending(session, null, "invitee@xata.io"))
    }

    @Test
    fun `pending is null without an email`() {
        assertNull(OrgInvitation.pending(session, authSession(null), null))
    }

    @Test
    fun `pending is null when the email was typed on the review profile form`() {
        val authSession = authSession("true")

        assertNull(OrgInvitation.pending(session, authSession, "invitee@xata.io"))

        // The token is never read, so no unverified address can reach the membership check.
        verify(exactly = 0) { authSession.getClientNote(OrgInvitation.TOKEN_NOTE) }
    }

    @Test
    fun `pending reads the token when the provider vouched for the email`() {
        val authSession = authSession(null)

        assertNull(OrgInvitation.pending(session, authSession, "invitee@xata.io"))

        verify { authSession.getClientNote(OrgInvitation.TOKEN_NOTE) }
    }
}
