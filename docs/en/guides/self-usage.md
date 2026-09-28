# API Key self usage

Select **View usage with an API Key** on the admin sign-in page, or open `/self-usage` directly. Enter a **user** or **personal** API Key; no admin account is needed. Other key types cannot use this page.

The page shows the key name and the system time zone. You can switch languages and themes. The key is stored only in this browser tab's session storage: a refresh in the same tab keeps you signed in, while closing the tab or selecting **Log out** ends the page session. Logging out does not revoke the API Key; revoke it in the admin area if needed.

Choose today, yesterday, the last 7 days, the last 30 days, this month, or a custom range of at most 90 days (both dates included). Dates and request times follow the displayed system time zone; each daily total uses the request creation date. The summary lists successful and failed requests, tokens, and cost, with a per-model breakdown. The request log supports model and status filters and pagination. Processing requests may appear in the log but do not count as successful or failed. Tokens and costs from failed or canceled requests count if usage records were saved. A request with multiple usage records counts only once toward request totals.

Costs use the system currency. No usage records means zero cost; entirely unpriced usage shows “—”. Partially unpriced usage shows the known cost with an unpriced-record count, while a genuine zero price displays zero.

This page shows only **API-originated requests for this key in its project**. It excludes other keys, Playground, and test requests. The log omits request and response bodies, channels, upstream models, and error details. If sign-in or queries happen too frequently, the page asks you to try again later; restricted keys may also be denied access.
