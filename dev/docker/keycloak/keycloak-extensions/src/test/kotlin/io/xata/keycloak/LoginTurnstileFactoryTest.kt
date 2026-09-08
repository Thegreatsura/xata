package io.xata.keycloak

import org.junit.jupiter.api.Test
import org.keycloak.models.AuthenticationExecutionModel
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class LoginTurnstileFactoryTest {
    private val factory = LoginTurnstileFactory()

    @Test
    fun `getId returns correct provider ID`() {
        assertEquals("login-turnstile-action", factory.id)
    }

    @Test
    fun `getDisplayType returns correct display name`() {
        assertEquals("Turnstile Username Password Form", factory.displayType)
    }

    @Test
    fun `getReferenceCategory returns turnstile`() {
        assertEquals("turnstile", factory.referenceCategory)
    }

    @Test
    fun `isConfigurable returns true`() {
        assertTrue(factory.isConfigurable)
    }

    @Test
    fun `isUserSetupAllowed returns false`() {
        assertFalse(factory.isUserSetupAllowed)
    }

    @Test
    fun `getRequirementChoices contains REQUIRED`() {
        val choices = factory.requirementChoices
        assertEquals(1, choices.size)
        assertEquals(AuthenticationExecutionModel.Requirement.REQUIRED, choices[0])
    }

    @Test
    fun `getConfigProperties returns turnstile config properties`() {
        val properties = factory.configProperties
        assertNotNull(properties)
        assertTrue(properties.isNotEmpty())

        val propertyNames = properties.map { it.name }
        assertTrue(propertyNames.contains("site.key"))
        assertTrue(propertyNames.contains("secret"))
        assertTrue(propertyNames.contains("action"))
    }

    @Test
    fun `getHelpText returns descriptive text`() {
        val helpText = factory.helpText
        assertNotNull(helpText)
        assertTrue(helpText.contains("Turnstile"))
    }

    @Test
    fun `create returns LoginTurnstile instance`() {
        val authenticator = factory.create(null)
        assertNotNull(authenticator)
        assertTrue(authenticator is LoginTurnstile)
    }
}
