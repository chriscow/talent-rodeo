# Credential cleanup: October 19, 2026

Repository history was rewritten to remove exposed environment files and credential values. GitHub Actions is disabled for this archived project to prevent cleanup pushes from deploying code.

Use the blank environment templates for configuration names. Store populated configuration outside Git and exclude it from container build contexts.

Existing clones must use the rewritten history. Do not merge old branches, restore old Git bundles, or push old tags: doing so can reintroduce exposed objects. Preserve uncommitted work separately and reapply only reviewed changes onto the rewritten branches. Old commit links and signatures may no longer work.

Removing a file from Git does not revoke a credential. Provider-side revocation and rejection checks are tracked separately in the incident evidence. Third-party copies and GitHub cached or pull-request objects can require separate removal.
