package io.xata.keycloak

import com.fasterxml.jackson.databind.node.ObjectNode
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Test
import org.keycloak.models.GroupModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationModel
import org.keycloak.models.ProtocolMapperModel
import org.keycloak.models.UserModel
import org.keycloak.organization.OrganizationProvider
import org.keycloak.protocol.oidc.mappers.OIDCAttributeMapperHelper
import org.keycloak.util.JsonSerialization
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class OrganizationMembershipMapperTest {
    private fun model(
        included: String? = "billingStatus##usageTier",
        addRole: Boolean = false,
    ) = ProtocolMapperModel().apply {
        config =
            mutableMapOf(OIDCAttributeMapperHelper.TOKEN_CLAIM_NAME to "organization").also {
                if (included != null) it[OrganizationMembershipMapper.INCLUDED_ATTRIBUTES] = included
                if (addRole) it[OrganizationMembershipMapper.ADD_ROLE] = "true"
            }
    }

    private val user = mockk<UserModel>()

    private fun group(
        name: String,
        parent: String,
    ): GroupModel =
        mockk {
            every { this@mockk.name } returns name
            every { this@mockk.parent } returns mockk { every { this@mockk.name } returns parent }
        }

    // Groups are (name, parent name); a top-level group's parent is named after the organization ID.
    private fun provider(memberships: Map<String, List<Pair<String, String>>>): OrganizationProvider {
        val organizations =
            memberships.keys.associateWith { alias ->
                mockk<OrganizationModel> {
                    every { this@mockk.alias } returns alias
                    every { id } returns "id-$alias"
                }
            }
        return mockk {
            every { getByMember(user) } answers { organizations.values.stream() }
            organizations.forEach { (alias, organization) ->
                every { getOrganizationGroupsByMember(organization, user) } answers {
                    memberships.getValue(alias).map { (name, parent) -> group(name, parent) }.stream()
                }
            }
        }
    }

    private fun session(provider: OrganizationProvider): KeycloakSession =
        mockk { every { getProvider(OrganizationProvider::class.java) } returns provider }

    private fun mapped(
        json: String,
        provider: OrganizationProvider,
        model: ProtocolMapperModel = model(addRole = true),
    ) = trimmed(json, model).also { OrganizationMembershipMapper.addRoles(it, model, session(provider), user) }

    private fun role(
        claims: Map<String, Any>,
        alias: String,
    ) = (claims["organization"] as ObjectNode).path(alias).path(OrganizationMembershipMapper.ROLE)

    private fun roleOf(vararg groups: Pair<String, String>) =
        role(mapped("""{"acme": {"billingStatus": ["ok"]}}""", provider(mapOf("acme" to groups.toList()))), "acme")

    // The JSON claim type makes Keycloak store the claim as a Jackson tree.
    private fun claims(json: String): MutableMap<String, Any> = mutableMapOf("organization" to JsonSerialization.mapper.readTree(json))

    private fun trimmed(
        json: String,
        model: ProtocolMapperModel = model(),
    ) = claims(json).also { OrganizationMembershipMapper.trim(it, model) }

    @Test
    fun `keeps only the included attributes`() {
        val got =
            trimmed(
                """{"acme": {"billingStatus": ["ok"], "usageTier": ["t2"], "displayName": ["Acme"], "ssoPendingDomains": ["a.com"]}}""",
            )
        assertEquals(JsonSerialization.mapper.readTree("""{"acme": {"billingStatus": ["ok"], "usageTier": ["t2"]}}"""), got["organization"])
    }

    @Test
    fun `skips deleted organizations`() {
        val got =
            trimmed("""{"acme": {"billingStatus": ["ok"]}, "gone": {"billingStatus": ["ok"], "deletedAt": ["2026-09-01T00:00:00Z"]}}""")
        assertEquals(listOf("acme"), (got["organization"] as ObjectNode).fieldNames().asSequence().toList())
    }

    @Test
    fun `treats a blank deletion time as not deleted`() {
        val got = trimmed("""{"acme": {"billingStatus": ["ok"], "deletedAt": [" "]}}""")
        assertTrue((got["organization"] as ObjectNode).has("acme"))
    }

    @Test
    fun `drops the claim when every organization is deleted`() {
        val got = trimmed("""{"gone": {"deletedAt": ["2026-09-01T00:00:00Z"]}}""")
        assertFalse(got.containsKey("organization"))
    }

    @Test
    fun `keeps every attribute when none are listed`() {
        val json = """{"acme": {"billingStatus": ["ok"], "displayName": ["Acme"]}}"""
        assertEquals(JsonSerialization.mapper.readTree(json), trimmed(json, model(included = null))["organization"])
    }

    @Test
    fun `leaves an alias list alone`() {
        val aliases = listOf("acme", "globex")
        val claims = mutableMapOf<String, Any>("organization" to aliases)
        OrganizationMembershipMapper.trim(claims, model())
        assertEquals(aliases, claims["organization"])
    }

    @Test
    fun `offers the upstream settings plus the attribute list`() {
        val names = OrganizationMembershipMapper().configProperties.map { it.name }
        assertTrue("addOrganizationAttributes" in names)
        assertTrue(OrganizationMembershipMapper.INCLUDED_ATTRIBUTES in names)
    }

    @Test
    fun `adds no role when the setting is off`() {
        val provider = provider(mapOf("acme" to listOf("Admin" to "id-acme")))
        val got = mapped("""{"acme": {"billingStatus": ["ok"]}}""", provider, model())
        assertTrue(role(got, "acme").isMissingNode)
        verify(exactly = 0) { provider.getByMember(any()) }
    }

    @Test
    fun `adds the admin role`() {
        val got =
            mapped(
                """{"acme": {"billingStatus": ["ok"], "usageTier": ["t2"], "displayName": ["Acme"]}}""",
                provider(mapOf("acme" to listOf("Admin" to "id-acme"))),
            )
        assertEquals(
            JsonSerialization.mapper.readTree("""{"acme": {"billingStatus": ["ok"], "usageTier": ["t2"], "role": ["admin"]}}"""),
            got["organization"],
        )
    }

    @Test
    fun `adds the editor role`() {
        assertEquals(JsonSerialization.mapper.readTree("""["editor"]"""), roleOf("editor" to "id-acme"))
    }

    @Test
    fun `adds the viewer role`() {
        assertEquals(JsonSerialization.mapper.readTree("""["viewer"]"""), roleOf("VIEWER" to "id-acme"))
    }

    @Test
    fun `keeps the most privileged of several roles`() {
        assertEquals(
            JsonSerialization.mapper.readTree("""["admin"]"""),
            roleOf("Viewer" to "id-acme", "Admin" to "id-acme", "Editor" to "id-acme"),
        )
        assertEquals(JsonSerialization.mapper.readTree("""["editor"]"""), roleOf("Viewer" to "id-acme", "Editor" to "id-acme"))
    }

    @Test
    fun `ignores a reserved name in a subgroup`() {
        assertEquals(JsonSerialization.mapper.readTree("""["viewer"]"""), roleOf("Admin" to "Team", "Viewer" to "id-acme"))
    }

    @Test
    fun `adds an empty role without a reserved group`() {
        assertEquals(JsonSerialization.mapper.readTree("[]"), roleOf("Support" to "id-acme", "Admin" to "Support"))
    }

    @Test
    fun `adds a role per organization and none for a deleted one`() {
        val got =
            mapped(
                """{"acme": {"billingStatus": ["ok"]}, "globex": {"billingStatus": ["ok"]}, "gone": {"deletedAt": ["2026-09-01"]}}""",
                provider(
                    mapOf(
                        "acme" to listOf("Admin" to "id-acme"),
                        "globex" to listOf("Viewer" to "id-globex"),
                        "gone" to listOf("Admin" to "id-gone"),
                    ),
                ),
            )
        assertEquals(
            JsonSerialization.mapper.readTree(
                """{"acme": {"billingStatus": ["ok"], "role": ["admin"]}, "globex": {"billingStatus": ["ok"], "role": ["viewer"]}}""",
            ),
            got["organization"],
        )
    }

    @Test
    fun `offers the role setting off by default`() {
        val property = OrganizationMembershipMapper().configProperties.single { it.name == OrganizationMembershipMapper.ADD_ROLE }
        assertEquals("false", property.defaultValue)
    }
}
