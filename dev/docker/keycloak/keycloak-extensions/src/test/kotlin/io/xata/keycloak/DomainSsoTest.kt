package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.ModelValidationException
import org.keycloak.models.OrganizationDomainModel
import org.keycloak.models.OrganizationModel
import org.keycloak.organization.OrganizationProvider
import java.util.stream.Stream
import kotlin.test.assertEquals
import kotlin.test.assertNull

class DomainSsoTest {
    private fun broker(enabled: Boolean = true) =
        IdentityProviderModel().apply {
            alias = "sso-acme-acme-com"
            isEnabled = enabled
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
    inner class RoutingTest {
        private fun session(
            routes: List<OrganizationDomainModel>,
            linked: IdentityProviderModel = broker(),
        ): KeycloakSession {
            val organization =
                mockk<OrganizationModel> {
                    every { isEnabled } returns true
                    every { domains } answers { routes.stream() }
                    every { identityProviders } answers { Stream.of(linked) }
                }
            val provider =
                mockk<OrganizationProvider> {
                    every { isEnabled } returns true
                    every { hasOrganizations() } returns true
                    every { getByDomainName(any()) } returns organization
                }
            return mockk { every { getProvider(OrganizationProvider::class.java) } returns provider }
        }

        private fun routed(
            domain: String,
            autoRedirect: Boolean = true,
        ) = OrganizationDomainModel(domain, true, "sso-acme-acme-com", autoRedirect)

        @Test
        fun `takes the domain routed to the provider`() {
            val got = DomainSso.requiredBroker(session(listOf(routed("acme.com"))), "acme.com")
            assertEquals("sso-acme-acme-com", got?.alias)
        }

        @Test
        fun `leaves another domain of the same organization alone`() {
            val got = session(listOf(routed("acme.com"), OrganizationDomainModel("acme.io", true)))
            assertNull(DomainSso.requiredBroker(got, "acme.io"))
        }

        @Test
        fun `leaves the domain alone when the provider does not redirect on a match`() {
            val got = session(listOf(routed("acme.com", autoRedirect = false)))
            assertNull(DomainSso.requiredBroker(got, "acme.com"))
        }

        @Test
        fun `leaves a subdomain alone when it is held without a provider`() {
            val got = session(listOf(routed("*.acme.com"), OrganizationDomainModel("contractors.acme.com", true)))

            assertNull(DomainSso.requiredBroker(got, "contractors.acme.com"))
            assertEquals("sso-acme-acme-com", DomainSso.requiredBroker(got, "eng.acme.com")?.alias)
        }

        @Test
        fun `leaves the domain alone when its provider is disabled`() {
            val got = session(listOf(routed("acme.com")), linked = broker(enabled = false))
            assertNull(DomainSso.requiredBroker(got, "acme.com"))
        }
    }
}
