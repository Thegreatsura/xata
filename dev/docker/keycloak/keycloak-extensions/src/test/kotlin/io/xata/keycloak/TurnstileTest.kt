package io.xata.keycloak

import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.apache.http.HttpEntity
import org.apache.http.StatusLine
import org.apache.http.client.methods.CloseableHttpResponse
import org.apache.http.client.methods.HttpPost
import org.apache.http.impl.client.CloseableHttpClient
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import org.keycloak.forms.login.LoginFormsProvider
import java.io.ByteArrayInputStream
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class TurnstileTest {
    companion object {
        // Official Cloudflare Turnstile test keys
        // https://developers.cloudflare.com/turnstile/troubleshooting/testing/
        const val TEST_SITE_KEY = "1x00000000000000000000AA" // Always passes (visible)
        const val TEST_SECRET_KEY = "1x0000000000000000000000000000000AA" // Always passes
        const val TEST_SITE_KEY_FAIL = "2x00000000000000000000AB" // Always fails (visible)
        const val TEST_SECRET_KEY_FAIL = "2x0000000000000000000000000000000AA" // Always fails
        const val DUMMY_TOKEN = "XXXX.DUMMY.TOKEN.XXXX" // Dummy token for testing
    }

    @Nested
    inner class ReadConfigTest {
        @Test
        fun `returns null when site key is missing`() {
            val config = mapOf("secret" to TEST_SECRET_KEY)
            val result = Turnstile.readConfig(config, "default-action")
            assertNull(result)
        }

        @Test
        fun `returns null when secret is missing`() {
            val config = mapOf("site.key" to TEST_SITE_KEY)
            val result = Turnstile.readConfig(config, "default-action")
            assertNull(result)
        }

        @Test
        fun `returns configuration with default action when action is missing`() {
            val config =
                mapOf(
                    "site.key" to TEST_SITE_KEY,
                    "secret" to TEST_SECRET_KEY,
                )
            val result = Turnstile.readConfig(config, "default-action")

            assertNotNull(result)
            assertEquals(TEST_SITE_KEY, result.siteKey)
            assertEquals(TEST_SECRET_KEY, result.secret)
            assertEquals("default-action", result.action)
        }

        @Test
        fun `returns configuration with custom action when provided`() {
            val config =
                mapOf(
                    "site.key" to TEST_SITE_KEY,
                    "secret" to TEST_SECRET_KEY,
                    "action" to "custom-action",
                )
            val result = Turnstile.readConfig(config, "default-action")

            assertNotNull(result)
            assertEquals(TEST_SITE_KEY, result.siteKey)
            assertEquals(TEST_SECRET_KEY, result.secret)
            assertEquals("custom-action", result.action)
        }

        @Test
        fun `returns null for empty config map`() {
            val result = Turnstile.readConfig(emptyMap(), "default-action")
            assertNull(result)
        }
    }

    @Nested
    inner class ValidateTest {
        private fun createMockHttpClient(
            statusCode: Int,
            responseBody: String,
        ): CloseableHttpClient {
            val httpClient = mockk<CloseableHttpClient>()
            val httpResponse = mockk<CloseableHttpResponse>()
            val statusLine = mockk<StatusLine>()
            val httpEntity = mockk<HttpEntity>()

            every { statusLine.statusCode } returns statusCode
            every { httpEntity.content } returns ByteArrayInputStream(responseBody.toByteArray())
            every { httpResponse.statusLine } returns statusLine
            every { httpResponse.entity } returns httpEntity
            every { httpResponse.close() } returns Unit
            every { httpClient.execute(any<HttpPost>()) } returns httpResponse

            return httpClient
        }

        @Test
        fun `returns true for successful validation with matching action`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient =
                createMockHttpClient(
                    200,
                    """{"success": true, "action": "login"}""",
                )

            val result = Turnstile.validate(config, DUMMY_TOKEN, "127.0.0.1", httpClient)

            assertTrue(result)
        }

        @Test
        fun `returns false when success is false`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient =
                createMockHttpClient(
                    200,
                    """{"success": false, "action": "login"}""",
                )

            val result = Turnstile.validate(config, DUMMY_TOKEN, "127.0.0.1", httpClient)

            assertFalse(result)
        }

        @Test
        fun `returns false when action does not match`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient =
                createMockHttpClient(
                    200,
                    """{"success": true, "action": "register"}""",
                )

            // Use non-dummy token to test action validation
            val result = Turnstile.validate(config, "real-token", "127.0.0.1", httpClient)

            assertFalse(result)
        }

        @Test
        fun `returns true for dummy token regardless of action`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient =
                createMockHttpClient(
                    200,
                    """{"success": true, "action": "different-action"}""",
                )

            // Cloudflare's dummy token format - action validation is skipped for test tokens
            val result =
                Turnstile.validate(
                    config,
                    DUMMY_TOKEN,
                    "127.0.0.1",
                    httpClient,
                )

            assertTrue(result)
        }

        @Test
        fun `returns false for non-200 status code`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient =
                createMockHttpClient(
                    500,
                    """{"success": true, "action": "login"}""",
                )

            val result = Turnstile.validate(config, DUMMY_TOKEN, "127.0.0.1", httpClient)

            assertFalse(result)
        }

        @Test
        fun `returns false when HTTP client throws exception`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient = mockk<CloseableHttpClient>()
            every { httpClient.execute(any<HttpPost>()) } throws RuntimeException("Connection failed")

            val result = Turnstile.validate(config, DUMMY_TOKEN, "127.0.0.1", httpClient)

            assertFalse(result)
        }

        @Test
        fun `sends correct parameters to Cloudflare API`() {
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "login")
            val httpClient = mockk<CloseableHttpClient>()
            val httpResponse = mockk<CloseableHttpResponse>()
            val statusLine = mockk<StatusLine>()
            val httpEntity = mockk<HttpEntity>()
            val postSlot = slot<HttpPost>()

            every { statusLine.statusCode } returns 200
            every { httpEntity.content } returns
                ByteArrayInputStream(
                    """{"success": true, "action": "login"}""".toByteArray(),
                )
            every { httpResponse.statusLine } returns statusLine
            every { httpResponse.entity } returns httpEntity
            every { httpResponse.close() } returns Unit
            every { httpClient.execute(capture(postSlot)) } returns httpResponse

            Turnstile.validate(config, DUMMY_TOKEN, "192.168.1.1", httpClient)

            verify { httpClient.execute(any<HttpPost>()) }
            assertEquals(
                "https://challenges.cloudflare.com/turnstile/v0/siteverify",
                postSlot.captured.uri.toString(),
            )
        }
    }

    @Nested
    inner class PrepareFormTest {
        @Test
        fun `adds turnstile script and attributes to form`() {
            val form = mockk<LoginFormsProvider>()
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "test-action")

            every { form.addScript(any<String>()) } answers { form }
            every { form.setAttribute(any<String>(), any()) } answers { form }

            val result = Turnstile.prepareForm(form, config, "en-US")

            verify { form.addScript("https://challenges.cloudflare.com/turnstile/v0/api.js") }
            verify { form.setAttribute("captchaRequired", true) }
            verify { form.setAttribute("captchaSiteKey", TEST_SITE_KEY) }
            verify { form.setAttribute("captchaAction", "test-action") }
            verify { form.setAttribute("captchaLanguage", "en-US") }
            assertTrue(result === form)
        }

        @Test
        fun `handles null config gracefully`() {
            val form = mockk<LoginFormsProvider>()

            every { form.addScript(any<String>()) } answers { form }
            every { form.setAttribute(any<String>(), any()) } answers { form }

            val result = Turnstile.prepareForm(form, null, "en-US")

            verify { form.addScript("https://challenges.cloudflare.com/turnstile/v0/api.js") }
            verify { form.setAttribute("captchaRequired", true) }
            verify { form.setAttribute("captchaSiteKey", null) }
            verify { form.setAttribute("captchaAction", null) }
            verify { form.setAttribute("captchaLanguage", "en-US") }
            assertTrue(result === form)
        }

        @Test
        fun `handles null language`() {
            val form = mockk<LoginFormsProvider>()
            val config = Turnstile.Configuration(TEST_SITE_KEY, TEST_SECRET_KEY, "action")

            every { form.addScript(any<String>()) } answers { form }
            every { form.setAttribute(any<String>(), any()) } answers { form }

            Turnstile.prepareForm(form, config, null)

            verify { form.setAttribute("captchaLanguage", null) }
        }
    }
}
