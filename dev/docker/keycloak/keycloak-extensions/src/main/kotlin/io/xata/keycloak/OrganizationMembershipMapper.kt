package io.xata.keycloak

import com.fasterxml.jackson.databind.JsonNode
import com.fasterxml.jackson.databind.node.ObjectNode
import org.keycloak.models.ClientSessionContext
import org.keycloak.models.Constants
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationModel
import org.keycloak.models.ProtocolMapperModel
import org.keycloak.models.UserModel
import org.keycloak.models.UserSessionModel
import org.keycloak.organization.OrganizationProvider
import org.keycloak.protocol.oidc.mappers.OIDCAttributeMapperHelper
import org.keycloak.provider.ProviderConfigProperty
import org.keycloak.representations.IDToken
import org.keycloak.organization.protocol.mappers.oidc.OrganizationMembershipMapper as KeycloakOrganizationMembershipMapper

// Keycloak's mapper copies every attribute of every membership, soft-deleted organizations included,
// and the frontend keeps the token in a cookie that outgrows request header limits.
class OrganizationMembershipMapper : KeycloakOrganizationMembershipMapper() {
    override fun getId() = PROVIDER_ID

    override fun getDisplayType() = "Xata Organization Membership"

    override fun getHelpText() = "Map user Organization membership, keeping only the listed attributes and skipping deleted organizations"

    override fun getConfigProperties(): List<ProviderConfigProperty> =
        super.getConfigProperties() +
            ProviderConfigProperty().apply {
                name = INCLUDED_ATTRIBUTES
                label = "Included attributes"
                helpText = "Organization attributes to keep in the claim. Empty keeps all of them."
                type = ProviderConfigProperty.MULTIVALUED_STRING_TYPE
            } +
            ProviderConfigProperty().apply {
                name = ADD_ROLE
                label = "Add role"
                helpText = "Add the Xata role the user holds in each organization, from its top-level Admin, Editor or Viewer group."
                type = ProviderConfigProperty.BOOLEAN_TYPE
                defaultValue = false.toString()
            }

    override fun setClaim(
        token: IDToken,
        model: ProtocolMapperModel,
        userSession: UserSessionModel,
        session: KeycloakSession,
        clientSessionCtx: ClientSessionContext,
    ) {
        super.setClaim(token, model, userSession, session, clientSessionCtx)
        trim(token.otherClaims, model)
        addRoles(token.otherClaims, model, session, userSession.user)
    }

    companion object {
        const val PROVIDER_ID = "xata-organization-membership-mapper"
        const val INCLUDED_ATTRIBUTES = "includedAttributes"
        const val ADD_ROLE = "addRole"
        const val ROLE = "role"

        // Most privileged first. Must match services/auth/roles.
        private val ROLES = listOf("admin", "editor", "viewer")

        // Must match OrganizationDeletedAtKey in services/auth/keycloak.
        const val DELETED_AT = "deletedAt"

        fun trim(
            claims: MutableMap<String, Any>,
            model: ProtocolMapperModel,
        ) {
            val name = model.config[OIDCAttributeMapperHelper.TOKEN_CLAIM_NAME] ?: return
            // Only the JSON form carries attributes; alias lists and nested claim names are left alone.
            val organizations = claims[name] as? ObjectNode ?: return
            val included =
                model.config[INCLUDED_ATTRIBUTES]
                    ?.split(Constants.CFG_DELIMITER)
                    ?.map(String::trim)
                    ?.filter(String::isNotEmpty)
                    .orEmpty()

            organizations.remove(
                organizations.properties().filter { (_, organization) -> isDeleted(organization) }.map { it.key },
            )
            if (included.isNotEmpty()) {
                organizations.forEach { (it as? ObjectNode)?.retain(included) }
            }
            if (organizations.isEmpty) claims.remove(name)
        }

        fun addRoles(
            claims: MutableMap<String, Any>,
            model: ProtocolMapperModel,
            session: KeycloakSession,
            user: UserModel,
        ) {
            if (!model.config[ADD_ROLE].toBoolean()) return
            val name = model.config[OIDCAttributeMapperHelper.TOKEN_CLAIM_NAME] ?: return
            val organizations = claims[name] as? ObjectNode ?: return
            val provider = session.getProvider(OrganizationProvider::class.java) ?: return
            val memberships = provider.getByMember(user).toList().associateBy { it.alias }

            organizations.properties().forEach { (alias, organization) ->
                val role = memberships[alias]?.let { roleOf(provider, it, user) }
                (organization as? ObjectNode)?.putArray(ROLE)?.apply { role?.let(::add) }
            }
        }

        private fun roleOf(
            provider: OrganizationProvider,
            organization: OrganizationModel,
            user: UserModel,
        ): String? {
            // A top-level organization group's parent is the internal group named after the organization ID.
            val held =
                provider
                    .getOrganizationGroupsByMember(organization, user)
                    .filter { it.parent?.name == organization.id }
                    .map { it.name.lowercase() }
                    .toList()
            return ROLES.firstOrNull { it in held }
        }

        private fun isDeleted(organization: JsonNode): Boolean {
            val deletedAt = organization.path(DELETED_AT)
            return (if (deletedAt.isArray) deletedAt.path(0) else deletedAt).asText().isNotBlank()
        }
    }
}
