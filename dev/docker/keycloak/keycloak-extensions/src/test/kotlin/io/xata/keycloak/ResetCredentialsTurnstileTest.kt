package io.xata.keycloak

import org.junit.jupiter.api.Test
import org.keycloak.models.AuthenticationExecutionModel
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class ResetCredentialsTurnstileTest {
    private val turnstile = ResetCredentialsTurnstile()

    @Test
    fun `getId returns correct provider ID`() {
        assertEquals("rescreds-user-turnstile", turnstile.id)
    }

    @Test
    fun `getDisplayType returns correct display name`() {
        assertEquals("Choose user (with turnstile challenge)", turnstile.displayType)
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
    fun `getRequirementChoices contains only REQUIRED`() {
        val choices = turnstile.requirementChoices
        assertEquals(1, choices.size)
        assertEquals(AuthenticationExecutionModel.Requirement.REQUIRED, choices[0])
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
        assertTrue(helpText.contains("reset credentials"))
    }
}
