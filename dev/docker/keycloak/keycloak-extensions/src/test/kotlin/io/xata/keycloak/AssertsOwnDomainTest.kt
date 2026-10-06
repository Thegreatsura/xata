package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Test
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.IdentityProviderStorageProvider
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationDomainModel
import org.keycloak.models.OrganizationModel
import org.keycloak.organization.OrganizationProvider
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * A provider bound to an organization domain is that organization's own software and can put any
 * address in a token. Keycloak's trustEmail believes it, and first broker login links on the
 * result, so without this check one customer's provider could claim another customer's account.
 */
class AssertsOwnDomainTest {
    private companion object {
        const val ORGANIZATION_ID = "acme"
    }

    private fun session(
        vararg providers: IdentityProviderModel,
        routes: List<OrganizationDomainModel> = emptyList(),
    ): KeycloakSession {
        val storage =
            mockk<IdentityProviderStorageProvider> {
                every { getByAlias(any()) } answers { providers.firstOrNull { it.alias == firstArg() } }
            }
        val organization =
            mockk<OrganizationModel> {
                every { isEnabled } returns true
                every { domains } answers { routes.stream() }
            }
        val organizations = mockk<OrganizationProvider> { every { getById(ORGANIZATION_ID) } returns organization }
        return mockk {
            every { identityProviders() } returns storage
            every { getProvider(OrganizationProvider::class.java) } returns organizations
        }
    }

    private fun linked(alias: String) =
        IdentityProviderModel().apply {
            this.alias = alias
            organizationIds = setOf(ORGANIZATION_ID)
        }

    private fun routed(domain: String) = listOf(OrganizationDomainModel(domain, true, "sso-acme-acme-com", false))

    @Test
    fun `accepts an address on the domain the provider is bound to`() {
        val got = session(linked("sso-acme-acme-com"), routes = routed("acme.com"))
        assertTrue(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "someone@acme.com"))
    }

    @Test
    fun `matches the domain case-insensitively`() {
        val got = session(linked("sso-acme-acme-com"), routes = routed("acme.com"))
        assertTrue(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "Someone@ACME.com"))
    }

    @Test
    fun `accepts a subdomain of a wildcard the provider is bound to`() {
        val got = session(linked("sso-acme-acme-com"), routes = routed("*.acme.com"))
        assertTrue(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "someone@eng.acme.com"))
    }

    @Test
    fun `refuses an address belonging to somebody else`() {
        val got = session(linked("sso-acme-acme-com"), routes = routed("acme.com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "victim@other.example"))
    }

    @Test
    fun `refuses a token carrying no address at all`() {
        val got = session(linked("sso-acme-acme-com"), routes = routed("acme.com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", null))
    }

    @Test
    fun `refuses every address from an organization's provider no domain routes to`() {
        val got = session(linked("sso-acme-acme-com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "victim@other.example"))
    }

    @Test
    fun `leaves a shared provider alone`() {
        // github and google are linked to no organization and are not this check's business.
        val got = session(IdentityProviderModel().apply { alias = "google" }, IdentityProviderModel().apply { alias = "github" })
        assertTrue(DomainSso.assertsOwnDomain(got, "google", "anyone@anywhere.example"))
        assertTrue(DomainSso.assertsOwnDomain(got, "github", "anyone@anywhere.example"))
    }

    @Test
    fun `refuses an organization provider no longer linked to any organization`() {
        val got = session(IdentityProviderModel().apply { alias = "sso-acme-acme-com" })
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "someone@acme.com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "someone@other.example"))
    }

    @Test
    fun `treats an unknown alias as not covered`() {
        assertTrue(DomainSso.assertsOwnDomain(session(), "gone", "someone@acme.com"))
    }
}
