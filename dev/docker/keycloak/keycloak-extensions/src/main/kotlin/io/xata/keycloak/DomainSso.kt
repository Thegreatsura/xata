package io.xata.keycloak

import org.jboss.logging.Logger
import org.keycloak.models.IdentityProviderModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.ModelValidationException
import org.keycloak.models.OrganizationDomainModel.ANY_DOMAIN
import org.keycloak.models.OrganizationModel
import org.keycloak.models.OrganizationModel.IdentityProviderRedirectMode.EMAIL_MATCH
import org.keycloak.organization.OrganizationProvider
import org.keycloak.organization.utils.Organizations

/**
 * Resolves the identity provider an organization redirects an email domain to.
 *
 * Mirrors the matching
 * [org.keycloak.organization.authentication.authenticators.browser.OrganizationAuthenticator] does
 * on the identity-first login page, so a domain resolves to the same provider however the user got
 * here.
 *
 * See [RequireDomainSso].
 */
object DomainSso {
    private val LOGGER: Logger = Logger.getLogger(DomainSso::class.java)

    /**
     * The provider the organization owning [domain] redirects that domain to, or null when no
     * organization claims the domain or none of its providers redirects on an email match.
     */
    fun requiredBroker(
        session: KeycloakSession,
        domain: String?,
    ): IdentityProviderModel? {
        if (domain == null) return null

        val organization = organizationFor(session, domain) ?: return null
        val matching = Organizations.getMatchingDomain(domain, organization) ?: return null

        return organization.identityProviders
            .filter { it.isEnabled && redirectsDomain(it, domain, matching.name) }
            .findFirst()
            .orElse(null)
    }

    /**
     * Whether [broker] takes [domain], which its organization holds as [organizationDomain].
     */
    fun redirectsDomain(
        broker: IdentityProviderModel,
        domain: String,
        organizationDomain: String,
    ): Boolean {
        if (!EMAIL_MATCH.isSet(broker)) return false

        val excluded = broker.config[OrganizationModel.ORGANIZATION_EXCLUDED_DOMAIN_ATTRIBUTE].orEmpty()
        if (excluded.split(',').any { Organizations.isSameDomain(domain, it.trim()) }) return false

        val brokerDomain = broker.config[OrganizationModel.ORGANIZATION_DOMAIN_ATTRIBUTE] ?: return false
        return brokerDomain == ANY_DOMAIN || brokerDomain == organizationDomain
    }

    private fun organizationFor(
        session: KeycloakSession,
        domain: String,
    ): OrganizationModel? {
        val provider = session.getProvider(OrganizationProvider::class.java)
        if (!Organizations.isEnabledAndOrganizationsPresent(provider)) return null

        // Ask only for the domain itself. getByDomainName widens the search on its own, adding
        // *.<domain> and each parent suffix when the exact name misses, and it stops short of the
        // bare TLD; handing it a wildcard we built ourselves reaches a name it validates and
        // rejects, which for an unclaimed domain is every login.
        return try {
            provider.getByDomainName(domain)?.takeIf { it.isEnabled }
        } catch (e: ModelValidationException) {
            // Nothing an organization could hold, so nobody redirects it. Refusing the login over
            // an address Keycloak will not parse is never the right answer here.
            LOGGER.debugf(e, "Not a usable email domain: '%s'", domain)
            null
        }
    }
}
