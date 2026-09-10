package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import jakarta.ws.rs.core.MultivaluedHashMap
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import org.keycloak.authentication.ValidationContext
import org.keycloak.http.HttpRequest
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.OrganizationDomainModel
import org.keycloak.models.OrganizationModel
import org.keycloak.models.OrganizationModel.IdentityProviderRedirectMode.EMAIL_MATCH
import org.keycloak.models.OrganizationModel.ORGANIZATION_DOMAIN_ATTRIBUTE
import org.keycloak.models.UserModel
import org.keycloak.models.utils.FormMessage
import org.keycloak.organization.OrganizationProvider
import java.util.stream.Stream
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * Covers the decision both SSO entry-point providers share. The flow plumbing around it needs a
 * running Keycloak, so it is left to the manual pass in the PR description.
 */
class RequireDomainSsoTest {
    private val ssoBroker =
        IdentityProviderModel().apply {
            alias = "google-sso-acme"
            displayName = "Acme SSO"
            isEnabled = true
            config = mutableMapOf(EMAIL_MATCH.key to "true", ORGANIZATION_DOMAIN_ATTRIBUTE to "acme.com")
        }

    /** A session whose realm holds one organization owning acme.com and redirecting it to [broker]. */
    private fun session(
        domain: String = "acme.com",
        broker: IdentityProviderModel? = ssoBroker,
        organizationEnabled: Boolean = true,
    ): KeycloakSession {
        val organization =
            mockk<OrganizationModel> {
                every { isEnabled } returns organizationEnabled
                every { domains } answers { Stream.of(OrganizationDomainModel(domain, true)) }
                every { identityProviders } answers { Stream.ofNullable(broker) }
            }
        val provider =
            mockk<OrganizationProvider> {
                every { isEnabled } returns true
                every { hasOrganizations() } returns true
                every { getByDomainName(any()) } answers { if (firstArg<String>() == domain) organization else null }
            }
        return mockk { every { getProvider(OrganizationProvider::class.java) } returns provider }
    }

    @Nested
    inner class MismatchedBrokerTest {
        @Test
        fun `holds a login on an SSO domain that has not been through the provider`() {
            val got = DomainSso.mismatchedBroker(session(), "alexis@acme.com", null)
            assertEquals("google-sso-acme", got?.alias)
        }

        @Test
        fun `lets a login that came back from the required provider through`() {
            assertNull(DomainSso.mismatchedBroker(session(), "alexis@acme.com", "google-sso-acme"))
        }

        @Test
        fun `holds a login that came back from a different provider`() {
            val got = DomainSso.mismatchedBroker(session(), "alexis@acme.com", "google")
            assertEquals("google-sso-acme", got?.alias)
        }

        @Test
        fun `leaves a domain no organization claims alone`() {
            assertNull(DomainSso.mismatchedBroker(session(), "someone@example.com", null))
        }

        @Test
        fun `leaves a login with no email alone`() {
            assertNull(DomainSso.mismatchedBroker(session(), null, null))
        }

        @Test
        fun `leaves a username that is not an email alone`() {
            assertNull(DomainSso.mismatchedBroker(session(), "alexis", null))
        }

        @Test
        fun `leaves the domain alone when the organization has no provider to redirect to`() {
            assertNull(DomainSso.mismatchedBroker(session(broker = null), "alexis@acme.com", null))
        }

        @Test
        fun `leaves the domain alone when the organization is disabled`() {
            val got = DomainSso.mismatchedBroker(session(organizationEnabled = false), "alexis@acme.com", null)
            assertNull(got)
        }

        @Test
        fun `matches the domain case-insensitively`() {
            val got = DomainSso.mismatchedBroker(session(), "Alexis@ACME.com", null)
            assertEquals("google-sso-acme", got?.alias)
        }
    }

    @Nested
    inner class RegistrationTest {
        private fun validate(email: String?): ValidationContext {
            val formData = MultivaluedHashMap<String, String>()
            email?.let { formData.add(UserModel.EMAIL, it) }

            val context =
                mockk<ValidationContext>(relaxed = true) {
                    every { httpRequest } returns mockk<HttpRequest> { every { decodedFormParameters } returns formData }
                    every { session } returns session()
                }
            RegistrationRequireDomainSso().validate(context)
            return context
        }

        @Test
        fun `refuses an address on an SSO domain, on the email field`() {
            val messages = slot<List<FormMessage>>()
            val context = validate("alexis@acme.com")

            verify { context.error(any()) }
            verify { context.validationError(any(), capture(messages)) }
            verify { context.excludeOtherErrors() }
            verify(exactly = 0) { context.success() }

            val message = messages.captured.single()
            assertEquals(UserModel.EMAIL, message.field)
            assertNotNull(message.message)
            assert(message.message.contains("Acme SSO")) { "expected the provider name, got ${message.message}" }
        }

        @Test
        fun `lets an address on any other domain register`() {
            val context = validate("someone@example.com")

            verify { context.success() }
            verify(exactly = 0) { context.validationError(any(), any()) }
        }

        @Test
        fun `leaves a blank email to the profile validator`() {
            val context = validate(null)

            verify { context.success() }
            verify(exactly = 0) { context.validationError(any(), any()) }
        }
    }
}
