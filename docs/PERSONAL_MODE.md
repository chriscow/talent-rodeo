# Personal mode

The web server defaults to full production. `--personal` selects a reduced,
single-owner job search without removing the full product's features.

The invocation for an already built server is:

```sh
/tmp/bin/web --env .prod.env --personal --personal-user-email 'YOUR_EXISTING_ACCOUNT_EMAIL'
```

`PERSONAL_USER_EMAIL` can supply the owner email instead of the second flag. The
email must identify an existing Firestore user. Owner resolution fails closed;
other users cannot sign in or use an existing session on this instance. The flag
applies to the web process, not standalone CLI or MCP commands.

Building and deploying the paused personal service can precede criteria review.
Keep the workers paused and leave the owner's search unconfirmed until that
review. Tests, live model requests, and functional job-search evaluation remain
part of the subsequent validation phase. Uploading a resume or confirming the
preferences form deliberately calls models even while background workers pause.

## Behavior

| Capability | Default production | `--personal` |
| --- | --- | --- |
| Accounts | Regular users | Configured owner only |
| Resume and dashboard | Enabled | Enabled |
| Search preferences | Interview onboarding | Written confirmation after resume |
| Job discovery, matching, feedback | Eligible subscribers | Owner with confirmed personal activation |
| Subscription eligibility | Paid/trial expiry | Separate personal marker; billing history unchanged |
| Inbox and Gmail health probe | Existing integration | Disabled |
| SMTP and email nudges | Existing integration | Disabled |
| Voice calls, appointment recovery, scheduled webhooks | Enabled | Disabled |
| Stripe routes/client | Enabled | Disabled |
| Voice object storage | Enabled | Not initialized |
| Admin and testing mutations | Existing production/test access rules | Routes absent |
| Pinecone startup configuration | Existing requirements | Not required |

The default production routes and navigation are restored from the sunset state.
Deploying without `--personal` therefore enables the full product's routes and
startup services. The Fly manifest explicitly selects `--personal`; the Docker
image's default command remains full production. `PERSONAL_USER_EMAIL` is supplied
as a Fly secret. The `Fly Deploy` GitHub Actions workflow automatically builds and
deploys pushes to `main`. Fly checks `/health` for readiness during the rollout.

Restored outbound call tasks carry an HMAC over the appointment/user request,
signed with the session secret. The handler reads and claims the stored scheduled
appointment and accepts calls only within its 15-minute due window. Old unsigned
tasks must be recreated deliberately. An ambiguous provider result leaves the
appointment claimed or failed for reconciliation instead of automatically dialing
again. Full voice operation still needs its own end-to-end validation, including
the existing local-time scheduling behavior.

Personal mode uses the existing Firestore and job records. It does not delete
historical tasks, appointments, subscriptions, or features. It processes only
search tasks belonging to the owner and created during the current confirmed
activation. A preferences confirmation atomically updates the owner's criteria,
statement version, and activation marker. Shared search criteria do not publish
personal results to historical subscribers.

The personal dashboard uses saved Firestore matches. Its active discovery and
matching path does not call Pinecone semantic search. Semantic search, embeddings,
and the separate CLI/MCP integrations remain in the code for future use.

## Services needed for personal use

- Firestore and its service-account credentials.
- Cloudflare KV for existing pause and schedule settings.
- Scrapingdog for fresh job listings and descriptions. The earlier account audit
  found zero available credits; this still needs resolution before live searches.
- OpenAI and DeepSeek for the active model calls.
- Pusher for progress updates, and existing Google/LinkedIn OAuth configuration.
- Session and job-ID encryption secrets, `BASE_URL`, and the existing Sentry setup.

Gmail, SMTP, Vapi, Twilio, Stripe, Pinecone, Gemini, and voice object-storage
credentials are not required by personal-mode startup. This does not validate the
remaining credentials or prove model access; those checks await criteria review.

The `search-scheduler`, `scrapingdog`, `subscription-processor`, and
`feedback-processor` KV settings still control their workers. Personal mode stays
paused when settings cannot be read and does not write enabling defaults. Existing
pause values have not been changed. It also preserves the configured processing
hours. Keep the full-production instance stopped if it shares this database: the
flag cannot constrain a second process running without it.

## Models and matching

All active GPT-4.1-mini call sites now use `gpt-6-luna`; DeepSeek Chat call sites use
`deepseek-flash` with thinking disabled. Both modes use the same model changes.
Historical cost names remain available to interpret old records. The local
`internal/llmtools` adapter supplies the request fields needed by the new model
names while retaining the existing prompt renderer and schema converter.

Provider errors, refusals, truncated output, and invalid match ratings must not
be recorded as completed low-quality matches. Personal evaluation metadata tracks
the inputs used for a rating so confirmed changes can trigger reevaluation.
Manually selected job statuses remain intact.
Older archived records do not distinguish automatic from manual archiving, so
reevaluation preserves their archived status. Newly auto-archived records carry
that distinction and can return to the feed if their updated rating improves.

Model cost figures are estimates: the current calculator does not model all
long-context tiers or provider-specific cache usage, and DeepSeek public-holiday
discounts require a calendar that is not included.

## Remaining restart sequence

1. Review Chris's target roles, seniority, geography, work arrangements,
   compensation preferences, must-haves, and exclusions. Update the resume if
   necessary. Distinguish firm constraints from preferences.
2. Run the source tests and build with isolated local fixtures/emulators; verify
   both mode policies, routes, templates, profile transitions, and task ownership.
3. Verify the active credentials and exact new model request formats with bounded
   requests. Confirm the deployed secret set independently of the local env file.
4. Restore Scrapingdog capacity, confirm the owner profile in the personal flow,
   and run a small controlled set of searches. Inspect relevance and failure paths.
5. Compare the frozen Flash baseline with a Jev shadow checker using complete,
   independently labeled listings and bounded evidence judgments. No Jev result
   should hide a job automatically during this comparison.
6. Verify the deployed personal mode, enabled workers, and processing hours, then
   enable only the intended KV settings. A scheduled worker
   rollout also needs a decision on Fly's current stop-on-idle behavior; the
   existing manifest allows zero running machines.

The personal inbox project is outside this restart.
