package io.xata.keycloak

import org.jboss.logging.Logger
import org.keycloak.Config
import org.keycloak.events.Details
import org.keycloak.events.Event
import org.keycloak.events.EventListenerProvider
import org.keycloak.events.EventListenerProviderFactory
import org.keycloak.events.EventType
import org.keycloak.events.admin.AdminEvent
import org.keycloak.models.AbstractKeycloakTransaction
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.models.utils.KeycloakModelUtils
import org.keycloak.organization.OrganizationProvider

// Temporary until an invitation can carry a role (https://github.com/keycloak/keycloak/issues/45238),
// e.g. through the invitation attributes of https://github.com/keycloak/keycloak/pull/48842.
class OrgInvitationRole(
    private val session: KeycloakSession,
) : EventListenerProvider {
    override fun onEvent(event: Event) {
        if (event.type != EventType.INVITE_ORG) return
        val realmId = event.realmId ?: return
        val userId = event.userId ?: return
        val orgId = event.details?.get(Details.ORG_ID) ?: return

        // After commit, so a failure cannot undo the join.
        session.transactionManager.enlistAfterCompletion(
            object : AbstractKeycloakTransaction() {
                override fun commitImpl() {
                    try {
                        KeycloakModelUtils.runJobInTransaction(session.keycloakSessionFactory) {
                            grant(it, realmId, orgId, userId)
                        }
                    } catch (e: Exception) {
                        LOGGER.errorf(e, "Could not grant the invited role to user %s in organization %s", userId, orgId)
                    }
                }

                override fun rollbackImpl() = Unit
            },
        )
    }

    override fun onEvent(
        event: AdminEvent,
        includeRepresentation: Boolean,
    ) = Unit

    override fun close() = Unit

    companion object {
        // Must match services/auth/roles.
        const val INVITED_ROLES = "invitedRoles"
        private const val VIEWER = "Viewer"
        private val ROLES = listOf("Admin", "Editor", VIEWER)
        private val LOGGER: Logger = Logger.getLogger(OrgInvitationRole::class.java)

        fun grant(
            session: KeycloakSession,
            realmId: String,
            orgId: String,
            userId: String,
        ) {
            val realm = session.realms().getRealm(realmId) ?: return
            session.context.realm = realm
            val provider = session.getProvider(OrganizationProvider::class.java) ?: return
            val organization = provider.getById(orgId) ?: return
            val user = session.users().getUserById(realm, userId) ?: return
            val email = user.email?.trim() ?: return

            val groups = provider.getTopLevelGroups(organization, null, null).toList()
            val viewer = groups.firstOrNull { it.name.equals(VIEWER, ignoreCase = true) } ?: return
            val entries = viewer.getAttributeStream(INVITED_ROLES).toList()
            val mine = entries.filter { it.substringBeforeLast('=').trim().equals(email, ignoreCase = true) }
            if (mine.isEmpty()) return

            val role = ROLES.firstOrNull { it.equals(mine.last().substringAfterLast('='), ignoreCase = true) }
            groups.firstOrNull { role != null && it.name.equals(role, ignoreCase = true) }?.let(user::joinGroup)

            val remaining = entries - mine.toSet()
            if (remaining.isEmpty()) viewer.removeAttribute(INVITED_ROLES) else viewer.setAttribute(INVITED_ROLES, remaining)
        }
    }
}

class OrgInvitationRoleFactory : EventListenerProviderFactory {
    companion object {
        const val PROVIDER_ID = "xata-org-invitation-role"
    }

    override fun create(session: KeycloakSession): EventListenerProvider = OrgInvitationRole(session)

    override fun init(config: Config.Scope?) = Unit

    override fun postInit(factory: KeycloakSessionFactory?) = Unit

    override fun close() = Unit

    override fun getId(): String = PROVIDER_ID
}
