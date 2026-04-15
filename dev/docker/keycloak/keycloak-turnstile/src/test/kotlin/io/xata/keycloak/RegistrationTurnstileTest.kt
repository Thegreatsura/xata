package io.xata.keycloak

import io.mockk.mockk
import org.junit.jupiter.api.Test
import org.keycloak.models.AuthenticationExecutionModel
import org.keycloak.models.KeycloakSession
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class RegistrationTurnstileTest {
    private val turnstile = RegistrationTurnstile()

    @Test
    fun `getId returns correct provider ID`() {
        assertEquals("registration-turnstile-action", turnstile.id)
    }

    @Test
    fun `getDisplayType returns Turnstile`() {
        assertEquals("Turnstile", turnstile.displayType)
    }

    @Test
    fun `getReferenceCategory returns turnstile`() {
        assertEquals("turnstile", turnstile.referenceCategory)
    }

    @Test
    fun `isConfigurable returns true`() {
        assertTrue(turnstile.isConfigurable)
    }

    @Test
    fun `isUserSetupAllowed returns false`() {
        assertFalse(turnstile.isUserSetupAllowed)
    }

    @Test
    fun `requiresUser returns false`() {
        assertFalse(turnstile.requiresUser())
    }

    @Test
    fun `getRequirementChoices contains REQUIRED and DISABLED`() {
        val choices = turnstile.requirementChoices
        assertEquals(2, choices.size)
        assertTrue(choices.contains(AuthenticationExecutionModel.Requirement.REQUIRED))
        assertTrue(choices.contains(AuthenticationExecutionModel.Requirement.DISABLED))
    }

    @Test
    fun `getConfigProperties returns turnstile config properties`() {
        val properties = turnstile.configProperties
        assertNotNull(properties)
        assertTrue(properties.isNotEmpty())

        val propertyNames = properties.map { it.name }
        assertTrue(propertyNames.contains("site.key"))
        assertTrue(propertyNames.contains("secret"))
        assertTrue(propertyNames.contains("action"))
    }

    @Test
    fun `getHelpText returns descriptive text`() {
        val helpText = turnstile.helpText
        assertNotNull(helpText)
        assertTrue(helpText.contains("Turnstile"))
        assertTrue(helpText.contains("human"))
    }

    @Test
    fun `create returns self`() {
        val mockSession = mockk<KeycloakSession>()
        val formAction = turnstile.create(mockSession)
        assertEquals(turnstile, formAction)
    }
}
