# FirePager mobile WebUI handoff

Updated 2026-11-01. This handoff records the phone-focused WebUI redesign and its production deployment so another agent can continue without repeating the work.

## Outcome

The WebUI has been redesigned for phone widths (including 390px): bottom tab navigation, tap-friendly call cards, full-screen call details, and compact admin receiver/user cards. Desktop tables and the existing API contracts remain in place.

## Changes

- `frontend/src/App.tsx`, `App.css`: fixed safe-area-aware bottom navigation for Operations, Admin (admins only), Profile, and Sign out; desktop top navigation retained.
- `frontend/src/hooks/useIsMobile.ts`: viewport breakpoint hook used to render phone cards in place of wide tables.
- `frontend/src/components/OperatorDashboard.tsx`: responsive call cards on phones and the existing sortable/scrollable table on desktop; detail drawer fills phone viewport.
- `frontend/src/components/AdminDashboard.tsx`: receiver fleet and user management render as readable cards on phones; desktop tables retained.
- `frontend/src/components/components.css`, `frontend/src/index.css`: consistent box sizing, prevent phone horizontal overflow, safe-area padding, minimum tap targets, responsive forms/cards/charts/drawers.
- `frontend/src/components/Profile.tsx`: notification choices wrap on small screens.
- `frontend/index.html`: FirePager title, mobile viewport-fit and theme metadata.

## Verification evidence

- `cd frontend && npm run build` — passed (`tsc -b` + Vite build).
- Scoped ESLint on changed UI components and `useIsMobile.ts` — passed. Repository-wide lint still reports pre-existing errors in `CallMap.tsx`, `DashboardCallMap.tsx`, `MessageFeed.tsx`, `Metrics.tsx`, `Profile.tsx`, `UserManager.tsx`, and `AuthContext.tsx`; the changed Profile line only changes notification layout.
- Playwright phone viewport 390×844 against a local Vite server with mocked API responses: `viewportWidth=390`, `documentWidth=390`, bottom navigation visible, 2 call cards and 2 receiver cards rendered, no horizontal overflow on Operations/Admin/Profile. Tapped a call card and verified the full-height detail sheet. Screenshots are in `/tmp/opencode/ui-check/` on the implementation host (not checked into Git).
- Screenshots used synthetic local admin state and mocked API responses; no production login was used. A temporary database preview user was deleted after an attempted live login; deletion returned `DELETE 1`.

## Deployment

- FirePager source commit: `3713cfd Make FirePager web UI phone friendly`, pushed to `fix/receiver-status-publisher-confirm`.
- GitHub Actions image workflow: [run 41364679073](https://github.com/ericgraf/firepager/actions/runs/41364679073), all component jobs succeeded.
- Frontend image pinned by digest: `ghcr.io/ericgraf/firepager/frontend@sha256:305ba54fdbb1271c6cdb66a9583683ee884fa17a4d3cd99b6fcc6922243ef105`.
- Flux GitOps commit: `5dcecf3 Deploy mobile-friendly FirePager frontend`, pushed to `flux-cloud` `main`.
- Manually reconciled `firepager-workloads`; Flux reported applied revision `refs/heads/main@sha1:5dcecf3234195d7ce15899c9814c3ef6acb76d5f`.
- Production rollout verified: `deployment/firepager-frontend successfully rolled out`, 1/1 Ready, image digest matches above.
- Production smoke test: `https://firepager.cloud.graffamily.ca/` returned HTTP 200 and served the new assets `index-3rYXO0r2.css` and `index-Cu9t7Uln.js`.

## Remaining notes

- An attempted direct local GHCR image push failed because the local token lacked `packages:write`; no deployment was made through that path. GitHub Actions successfully published the image with its configured permissions.
- Production authenticated visual QA was not completed; Playwright visual QA used fixture data. Browser should perform a final manual check on a phone, especially Profile subscription controls and Admin receiver cards.
- User assigned another bot to host-SDR tuning. Do not tune receiver frequencies/gains as part of this UI handoff. Continue SDR work from the existing receiver TODO and baseline records, coordinating with that bot.
- `TODO.md` also records the separate POCSAG field parsing issue (location, vehicles/trucks, station) and receiver decode discrepancy; those are not addressed by the UI deployment.
