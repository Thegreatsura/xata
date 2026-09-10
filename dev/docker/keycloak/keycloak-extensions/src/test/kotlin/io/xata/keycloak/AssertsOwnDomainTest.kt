package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Test
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.IdentityProviderStorageProvider
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationModel
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * A provider bound to an organization domain is that organization's own software and can put any
 * address in a token. Keycloak's trustEmail believes it, and first broker login links on the
 * result, so without this check one customer's provider could claim another customer's account.
 */
class AssertsOwnDomainTest {
    private fun session(vararg providers: IdentityProviderModel): KeycloakSession {
        val storage =
            mockk<IdentityProviderStorageProvider> {
                every { getByAlias(any()) } answers { providers.firstOrNull { it.alias == firstArg() } }
            }
        return mockk { every { identityProviders() } returns storage }
    }

    private fun bound(
        alias: String,
        domain: String?,
    ) = IdentityProviderModel().apply {
        this.alias = alias
        config =
            mutableMapOf<String, String>().also {
                if (domain != null) it[OrganizationModel.ORGANIZATION_DOMAIN_ATTRIBUTE] = domain
            }
    }

    @Test
    fun `accepts an address on the domain the provider is bound to`() {
        val got = session(bound("sso-acme-acme-com", "acme.com"))
        assertTrue(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "someone@acme.com"))
    }

    @Test
    fun `matches the domain case-insensitively`() {
        val got = session(bound("sso-acme-acme-com", "acme.com"))
        assertTrue(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "Someone@ACME.com"))
    }

    @Test
    fun `refuses an address belonging to somebody else`() {
        val got = session(bound("sso-acme-acme-com", "acme.com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", "victim@other.example"))
    }

    @Test
    fun `refuses a token carrying no address at all`() {
        val got = session(bound("sso-acme-acme-com", "acme.com"))
        assertFalse(DomainSso.assertsOwnDomain(got, "sso-acme-acme-com", null))
    }

    @Test
    fun `leaves a shared provider alone`() {
        // github and google are bound to no domain and are not this check's business.
        val got = session(bound("google", null))
        assertTrue(DomainSso.assertsOwnDomain(got, "google", "anyone@anywhere.example"))
    }

    @Test
    fun `treats an unknown alias as not covered`() {
        assertTrue(DomainSso.assertsOwnDomain(session(), "gone", "someone@acme.com"))
    }
}
