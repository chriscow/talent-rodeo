# Nudge Service: Safe Launch Plan

This document outlines a phased, safety-first approach to deploying and enabling the new Nudge email service. The primary goal is to prevent accidental mass emailing of users while thoroughly testing the system's logic in a production environment.

## Guiding Principles

- **Safety First**: Every step is designed to minimize risk.
- **Measure Everything**: We will monitor logs and metrics to validate behavior.
- **Gradual Rollout**: We will enable the feature progressively, not all at once.
- **Controlled Environment**: Use feature flags for fine-grained control at every stage.

---

## Phase 1: Silent Mode (Production Monitoring)

**Goal**: Verify the user eligibility logic and processing cycle in production without sending any emails.

**Duration**: 1-2 days

### Actions:
1.  **Deploy Code**: Deploy the nudge processor code to production.
2.  **Configure Feature Flags**:
    -   `enabled`: `true` (to run the processor)
    -   `dry_run`: `true` (to prevent email sending)
    -   `test_mode_enabled`: `false`
    -   `paused`: `false`
3.  **Set a high `Interval`**: Start with a longer processing interval (e.g., `1h`) to reduce noise.

### Monitoring & Validation:
-   **Log Analysis**:
    -   Monitor structured logs for `starting nudge processing cycle`.
    -   Check logs for `found eligible users` to see how many users are being selected for each nudge type.
    -   Verify `DRY RUN: would send nudge email` logs to confirm that the processor is correctly identifying users and generating email content.
-   **Metrics**:
    -   Track the `EligibleUsers` count in `TypeStats`. Does it match expectations?
    -   Ensure `EmailsSent` count in `TypeStats` is always `0`.
-   **Alerts**:
    -   Set up an alert for any errors logged by the `nudge_processor` component.

**Success Criteria**: The processor runs without errors, and the number of eligible users identified in logs seems reasonable and aligns with manual checks against the database.

---

## Phase 2: Internal Testing (Dogfooding)

**Goal**: Test the end-to-end email sending flow with a small, controlled group of internal test accounts.

**Duration**: 1 day

### Actions:
1.  **Create Test Accounts**: Create several test user accounts in the production database with various states (e.g., `pending_resume_upload`, `resume_uploaded`). Use a specific, non-production email domain (e.g., `chris@test.talentrodeo.com`).
2.  **Configure Feature Flags**:
    -   `dry_run`: `false` (to enable actual email sending)
    -   `test_mode_enabled`: `true`
    -   `require_test_domain`: `true`
    -   `test_domains`: `["test.talentrodeo.com"]` (or your chosen internal domain)
    -   `max_emails_per_run`: `5` (low limit)
    -   `max_test_emails_total`: `20` (a hard cap for the test)

### Monitoring & Validation:
-   **Email Verification**:
    -   Confirm that test accounts receive the correct nudge emails.
    -   Verify email content, personalization, and formatting.
-   **Database Checks**:
    -   After an email is sent, check the corresponding user document in Firestore.
    -   Ensure `LastNudgeEmailSent`, `LastNudgeEmailType`, and `NudgeEmailsSentCount` are updated correctly.
-   **Log Analysis**:
    -   Monitor logs for `successfully sent nudge email`.
    -   Ensure no logs appear for `email not safe to send`.

**Success Criteria**: The system sends emails only to the specified test accounts. The tracking fields in Firestore are updated correctly, and rate-limiting prevents immediate re-sending of nudges.

---

## Phase 3: Limited Beta (First Real Users)

**Goal**: Roll out the feature to a small percentage of real users to observe its impact and performance at a small scale.

**Duration**: 2-3 days

### Actions:
1.  **Configure Feature Flags**:
    -   `test_mode_enabled`: `false` (disable test-specific checks)
    -   `max_emails_per_run`: `10` (a conservative, low number)
    -   `interval`: `30m` (a more frequent interval)

### Monitoring & Validation:
-   **User Metrics**:
    -   Monitor user engagement metrics. Are users who receive nudges returning to the platform?
    -   Watch for any increase in user complaints or support tickets.
-   **System Performance**:
    -   Monitor Firestore read/write operations and latency.
    -   Keep an eye on the performance of the LLM calls for email generation.
-   **Email Deliverability**:
    -   Check SMTP provider stats for open rates, bounce rates, and spam complaints.

**Success Criteria**: The system operates smoothly with real users. Engagement metrics show a neutral or positive impact. No significant performance degradation or user complaints.

---

## Phase 4: Full Launch (General Availability)

**Goal**: Make the nudge service available to all eligible users.

**Duration**: Ongoing

### Actions:
1.  **Configure Feature Flags**:
    -   Gradually increase the `max_emails_per_run` setting to its final value (e.g., from `10` to `25` to `50`).
    -   Set the `Interval` to its final intended value (e.g., `5m` or `15m`).

### Monitoring & Validation:
-   **Continuous Monitoring**: Continue monitoring all the metrics and logs from the previous phases.
-   **A/B Testing (Optional)**: For more advanced analysis, consider a framework where a control group of users is explicitly excluded from nudges to measure the feature's lift on engagement metrics accurately.
-   **Feedback Loop**: Continue to gather user feedback and iterate on the email content and nudge logic.

**Success Criteria**: The nudge service runs reliably as a background process, demonstrably improving user engagement and retention without causing negative side effects.

This phased approach ensures that you can build confidence in the system at each step, with safety nets in place to catch any unexpected behavior before it affects a large number of users. 