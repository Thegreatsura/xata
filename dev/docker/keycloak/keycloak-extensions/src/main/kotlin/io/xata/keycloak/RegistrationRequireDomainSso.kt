package io.xata.keycloak

import org.keycloak.Config
import org.keycloak.authentication.FormAction
import org.keycloak.authentication.FormActionFactory
import org.keycloak.authentication.FormContext
import org.keycloak.authentication.ValidationContext
import org.keycloak.events.Errors
import org.keycloak.forms.login.LoginFormsProvider
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.KeycloakSession
import org.keycloak.models.KeycloakSessionFactory
import org.keycloak.models.RealmModel
import org.keycloak.models.UserModel
import org.keycloak.models.utils.FormMessage
import org.keycloak.provider.ProviderConfigProperty

/**
 * Refuses a registration whose email domain belongs to an organization with its own provider.
 *
 * The registration form is a form flow, so it takes a [FormAction] rather than the authenticator
 * [RequireDomainSso]. Belongs before the password action, so the account is refused before any
 * credential is created.
 *
 * See [DomainSso].
 */
class RegistrationRequireDomainSso : FormAction, FormActionFactory {
    companion object {
        const val PROVIDER_ID = "xata-registration-require-domain-sso"
    }

    override fun create(session: KeycloakSession): FormAction = this

    override fun init(config: Config.Scope) = Unit

    override fun postInit(factory: KeycloakSessionFactory) = Unit

    override fun close() = Unit

    override fun buildPage(
        context: FormContext,
        form: LoginFormsProvider,
    ) = Unit

    override fun validate(context: ValidationContext) {
        val formData = context.httpRequest.decodedFormParameters
        val email = formData.getFirst(UserModel.EMAIL)
        val required = DomainSso.mismatchedBroker(context.session, email, null)

        if (required == null) {
            context.success()
            return
        }

        context.error(Errors.INVALID_REGISTRATION)
        context.validationError(
            formData,
            listOf(
                FormMessage(
                    UserModel.EMAIL,
                    DomainSso.useYourProvider(required, "Use that instead of registering."),
                ),
            ),
        )
        context.excludeOtherErrors()
    }

    override fun success(context: FormContext) = Unit

    override fun requiresUser(): Boolean = false

    override fun configuredFor(
        session: KeycloakSession,
        realm: RealmModel,
        user: UserModel,
    ): Boolean = true

    override fun setRequiredActions(
        session: KeycloakSession,
        realm: RealmModel,
        user: UserModel,
    ) = Unit

    override fun getId(): String = PROVIDER_ID

    override fun getDisplayType(): String = "Require Organization SSO For Email Domain (Registration)"

    override fun getHelpText(): String =
        "Refuses a registration when the email domain belongs to an organization that redirects it " +
            "to its own identity provider."

    override fun getReferenceCategory(): String = "organization"

    override fun isConfigurable(): Boolean = false

    override fun getConfigProperties(): MutableList<ProviderConfigProperty> = mutableListOf()

    override fun getRequirementChoices(): Array<AuthenticationExecutionModel.Requirement> =
        arrayOf(
            AuthenticationExecutionModel.Requirement.REQUIRED,
            AuthenticationExecutionModel.Requirement.DISABLED,
        )

    override fun isUserSetupAllowed(): Boolean = false
}
