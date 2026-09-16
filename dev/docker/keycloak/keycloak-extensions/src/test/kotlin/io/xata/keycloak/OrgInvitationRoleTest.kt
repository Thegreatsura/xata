package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Test
import org.keycloak.events.Details
import org.keycloak.events.Event
import org.keycloak.events.EventType
import org.keycloak.models.GroupModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationModel
import org.keycloak.models.RealmModel
import org.keycloak.models.UserModel
import org.keycloak.organization.OrganizationProvider
import java.util.stream.Stream

class OrgInvitationRoleTest {
    private fun group(
        name: String,
        vararg invited: String,
    ): GroupModel =
        mockk(relaxed = true) {
            every { this@mockk.name } returns name
            every { getAttributeStream(OrgInvitationRole.INVITED_ROLES) } answers { Stream.of(*invited) }
        }

    private fun user(email: String = "invitee@xata.io"): UserModel =
        mockk(relaxed = true) {
            every { this@mockk.email } returns email
        }

    private fun session(
        user: UserModel,
        vararg groups: GroupModel,
    ): KeycloakSession {
        val realm = mockk<RealmModel>()
        val organization = mockk<OrganizationModel>()
        val provider =
            mockk<OrganizationProvider> {
                every { getById("org") } returns organization
                every { getTopLevelGroups(organization, null, null) } answers { Stream.of(*groups) }
            }
        return mockk(relaxed = true) {
            every { realms().getRealm("realm") } returns realm
            every { users().getUserById(realm, "user") } returns user
            every { getProvider(OrganizationProvider::class.java) } returns provider
        }
    }

    @Test
    fun `joins the recorded role and removes the entry`() {
        val editor = group("Editor")
        val viewer = group("Viewer", "someone@xata.io=admin", "invitee@xata.io=editor")
        val user = user()

        OrgInvitationRole.grant(session(user, group("Admin"), editor, viewer), "realm", "org", "user")

        verify { user.joinGroup(editor) }
        verify { viewer.setAttribute(OrgInvitationRole.INVITED_ROLES, listOf("someone@xata.io=admin")) }
    }

    @Test
    fun `matches the address case-insensitively`() {
        val admin = group("admin")
        val viewer = group("VIEWER", "Invitee@Xata.io=Admin")
        val user = user("invitee@XATA.io")

        OrgInvitationRole.grant(session(user, admin, viewer), "realm", "org", "user")

        verify { user.joinGroup(admin) }
        verify { viewer.removeAttribute(OrgInvitationRole.INVITED_ROLES) }
    }

    @Test
    fun `does nothing without an entry`() {
        val viewer = group("Viewer", "someone@xata.io=admin")
        val user = user()

        OrgInvitationRole.grant(session(user, group("Admin"), viewer), "realm", "org", "user")

        verify(exactly = 0) { user.joinGroup(any()) }
        verify(exactly = 0) { viewer.setAttribute(any(), any()) }
        verify(exactly = 0) { viewer.removeAttribute(any()) }
    }

    @Test
    fun `ignores an unknown role and removes its entry`() {
        val owner = group("owner")
        val viewer = group("Viewer", "invitee@xata.io=owner")
        val user = user()

        OrgInvitationRole.grant(session(user, owner, viewer), "realm", "org", "user")

        verify(exactly = 0) { user.joinGroup(any()) }
        verify { viewer.removeAttribute(OrgInvitationRole.INVITED_ROLES) }
    }

    @Test
    fun `waits for the membership to commit`() {
        val session = mockk<KeycloakSession>(relaxed = true)
        val event =
            Event().apply {
                type = EventType.INVITE_ORG
                realmId = "realm"
                userId = "user"
                details = mapOf(Details.ORG_ID to "org")
            }

        OrgInvitationRole(session).onEvent(event)

        verify { session.transactionManager.enlistAfterCompletion(any()) }
        verify(exactly = 0) { session.keycloakSessionFactory }
    }

    @Test
    fun `ignores a failed acceptance`() {
        val session = mockk<KeycloakSession>(relaxed = true)
        val event =
            Event().apply {
                type = EventType.INVITE_ORG_ERROR
                realmId = "realm"
                userId = "user"
                details = mapOf(Details.ORG_ID to "org")
            }

        OrgInvitationRole(session).onEvent(event)

        verify(exactly = 0) { session.transactionManager }
    }
}
