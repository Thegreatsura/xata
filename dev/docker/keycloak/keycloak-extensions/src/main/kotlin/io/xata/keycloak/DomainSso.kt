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
 * Resolves the identity provider an organization redirects an email domain to, matching what
 * [org.keycloak.organization.authentication.authenticators.browser.OrganizationAuthenticator] does
 * on the identity-first login page.
 *
 * See [RequireDomainSso].
 */
object DomainSso {
    private val LOGGER: Logger = Logger.getLogger(DomainSso::class.java)

    /** What to tell somebody whose address belongs to [required], and what to do instead. */
    fun useYourProvider(
        required: IdentityProviderModel,
        then: String,
    ): String {
        val name = required.displayName.orEmpty().ifBlank { required.alias }
        return "Your organization signs in through $name. $then"
    }

    /** The provider the organization owning [domain] redirects it to, or null if none does. */
    fun requiredBroker(
        session: KeycloakSession,
        domain: String?,
    ): IdentityProviderModel? {
        val canonical = domain?.lowercase() ?: return null

        val organization = organizationFor(session, canonical) ?: return null
        val matching = Organizations.getMatchingDomain(canonical, organization) ?: return null

        return organization.identityProviders
            .filter { it.isEnabled && redirectsDomain(it, canonical, matching.name) }
            .findFirst()
            .orElse(null)
    }

    /**
     * The provider a login by [email] has to be held for, or null when none does or [currentAlias]
     * already is it. [currentAlias] is null outside a broker flow, where nothing has been proven.
     */
    fun mismatchedBroker(
        session: KeycloakSession,
        email: String?,
        currentAlias: String?,
    ): IdentityProviderModel? {
        val required = requiredBroker(session, Organizations.getEmailDomain(email)) ?: return null
        return if (required.alias == currentAlias) null else required
    }

    /** Whether [broker] takes [domain], which its organization holds as [organizationDomain]. */
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

    /**
     * Whether [alias] is entitled to assert [email]. Keycloak's trustEmail does not check this and
     * first broker login links on the result, so without it one organization's provider could
     * claim another's account. A provider bound to no domain is shared and not covered.
     */
    fun assertsOwnDomain(
        session: KeycloakSession,
        alias: String?,
        email: String?,
    ): Boolean {
        if (alias == null) return true
        val idp = session.identityProviders().getByAlias(alias) ?: return true
        val bound = idp.config[OrganizationModel.ORGANIZATION_DOMAIN_ATTRIBUTE] ?: return true

        val domain = Organizations.getEmailDomain(email)?.lowercase() ?: return false
        if (bound != ANY_DOMAIN) return Organizations.isSameDomain(domain, bound)

        val organization = organizationFor(session, domain) ?: return false
        return organization.identityProviders.anyMatch { it.alias == alias }
    }

    private fun organizationFor(
        session: KeycloakSession,
        domain: String,
    ): OrganizationModel? {
        val provider = session.getProvider(OrganizationProvider::class.java)
        if (!Organizations.isEnabledAndOrganizationsPresent(provider)) return null

        // getByDomainName widens the search itself and rejects a wildcard on a bare TLD, so ask
        // it for the domain and nothing else.
        return try {
            provider.getByDomainName(domain)?.takeIf { it.isEnabled }
        } catch (e: ModelValidationException) {
            LOGGER.debugf(e, "Not a usable email domain: '%s'", domain)
            null
        }
    }
}
