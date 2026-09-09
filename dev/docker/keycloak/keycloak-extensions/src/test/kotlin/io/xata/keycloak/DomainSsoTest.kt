package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.ModelValidationException
import org.keycloak.models.OrganizationDomainModel.ANY_DOMAIN
import org.keycloak.models.OrganizationModel
import org.keycloak.models.OrganizationModel.IdentityProviderRedirectMode.EMAIL_MATCH
import org.keycloak.models.OrganizationModel.ORGANIZATION_DOMAIN_ATTRIBUTE
import org.keycloak.models.OrganizationModel.ORGANIZATION_EXCLUDED_DOMAIN_ATTRIBUTE
import org.keycloak.organization.OrganizationProvider
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class DomainSsoTest {
    private fun broker(vararg config: Pair<String, String>) =
        IdentityProviderModel().apply {
            alias = "google-sso-acme"
            this.config = mutableMapOf(EMAIL_MATCH.key to "true", *config)
        }

    @Nested
    inner class RequiredBrokerTest {
        private fun session(provider: OrganizationProvider): KeycloakSession =
            mockk {
                every { getProvider(OrganizationProvider::class.java) } returns provider
            }

        private fun provider(block: OrganizationProvider.() -> Unit): OrganizationProvider =
            mockk<OrganizationProvider> {
                every { isEnabled } returns true
                every { hasOrganizations() } returns true
                block()
            }

        @Test
        fun `asks only for the domain, because getByDomainName widens the search itself`() {
            val got = provider { every { getByDomainName(any()) } returns null }

            assertNull(DomainSso.requiredBroker(session(got), "eng.acme.com"))

            verify(exactly = 1) { got.getByDomainName("eng.acme.com") }
            verify(exactly = 0) { got.getByDomainName("*.acme.com") }
            verify(exactly = 0) { got.getByDomainName("*.com") }
        }

        @Test
        fun `leaves a login alone when the domain is one Keycloak refuses to look up`() {
            // getByDomainName validates before it queries. An address it will not parse belongs to
            // no organization; it must not take the login down with it.
            val got =
                provider {
                    every { getByDomainName(any()) } throws ModelValidationException("Invalid domain format")
                }

            assertNull(DomainSso.requiredBroker(session(got), "not a domain"))
        }

        @Test
        fun `ignores an organization that is disabled`() {
            val organization = mockk<OrganizationModel> { every { isEnabled } returns false }
            val got = provider { every { getByDomainName("acme.com") } returns organization }

            assertNull(DomainSso.requiredBroker(session(got), "acme.com"))
        }

        @Test
        fun `finds no provider when organizations are not in use`() {
            val got =
                mockk<OrganizationProvider> {
                    every { isEnabled } returns true
                    every { hasOrganizations() } returns false
                }

            assertNull(DomainSso.requiredBroker(session(got), "acme.com"))
        }
    }

    @Nested
    inner class RedirectsDomainTest {
        @Test
        fun `takes the domain it is configured for`() {
            val got = broker(ORGANIZATION_DOMAIN_ATTRIBUTE to "acme.com")
            assertTrue(DomainSso.redirectsDomain(got, "acme.com", "acme.com"))
        }

        @Test
        fun `takes any domain of the organization`() {
            val got = broker(ORGANIZATION_DOMAIN_ATTRIBUTE to ANY_DOMAIN)
            assertTrue(DomainSso.redirectsDomain(got, "acme.com", "acme.com"))
        }

        @Test
        fun `leaves another domain of the same organization alone`() {
            val got = broker(ORGANIZATION_DOMAIN_ATTRIBUTE to "acme.com")
            assertFalse(DomainSso.redirectsDomain(got, "acme.io", "acme.io"))
        }

        @Test
        fun `leaves the domain alone when it is excluded`() {
            val got =
                broker(
                    ORGANIZATION_DOMAIN_ATTRIBUTE to ANY_DOMAIN,
                    ORGANIZATION_EXCLUDED_DOMAIN_ATTRIBUTE to "contractors.acme.com, partners.acme.com",
                )
            assertFalse(DomainSso.redirectsDomain(got, "contractors.acme.com", "acme.com"))
            assertTrue(DomainSso.redirectsDomain(got, "acme.com", "acme.com"))
        }

        @Test
        fun `leaves the domain alone when a wildcard excludes it`() {
            val got =
                broker(
                    ORGANIZATION_DOMAIN_ATTRIBUTE to ANY_DOMAIN,
                    ORGANIZATION_EXCLUDED_DOMAIN_ATTRIBUTE to "*.dev.acme.com",
                )
            assertFalse(DomainSso.redirectsDomain(got, "eu.dev.acme.com", "acme.com"))
        }

        @Test
        fun `leaves the domain alone when the provider does not redirect on a match`() {
            val got =
                IdentityProviderModel().apply {
                    config = mutableMapOf(ORGANIZATION_DOMAIN_ATTRIBUTE to "acme.com")
                }
            assertFalse(DomainSso.redirectsDomain(got, "acme.com", "acme.com"))
        }

        @Test
        fun `leaves the domain alone when the provider is not bound to one`() {
            assertFalse(DomainSso.redirectsDomain(broker(), "acme.com", "acme.com"))
        }
    }
}
