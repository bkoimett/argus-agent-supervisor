# Argus
### Monetization Plan

Version 1.0

---

## 1. Principle

Monetization must never compromise the core loop. The product stays genuinely useful for free, for a single developer, on a single machine, indefinitely. Paying should unlock more scale and more convenience, not basic reliability.

## 2. Free tier

- One connected computer
- Up to five concurrent agent sessions
- Full remote control and notifications
- Basic quota display where the provider exposes it
- Limited history (for example, the last 7 days of session activity)

This tier alone should be enough to fully validate the product before a single dollar changes hands.

## 3. Pro tier

Target price: $5 to $10 a month, not fixed until validated with real users who have used the free tier for at least a few weeks.

Candidate features, added only once each is actually built and proven useful in the free tier's daily use:

- More than one connected computer
- More than five concurrent sessions
- Extended history
- Resource Guardian (automatic pause and protection rules)
- Model fallback routing
- Advanced notification rules

Do not price Pro around features that only exist on the roadmap. Price it around features that exist and that free users have already asked for more of.

## 4. Cost structure

| Stage | Infrastructure cost | Notes |
|---|---|---|
| Solo use / early beta | $0/month | Supabase free tier, Web Push or ntfy.sh, SQLite locally |
| Small beta (dozens of users) | $0/month | Still inside Supabase free tier limits in most cases |
| Early paid users (low hundreds) | $0 to $25/month | Supabase's first paid tier if free tier limits are hit, or self-hosted Postgres on a small VPS |
| Growth stage | Scales with usage | Revisit infrastructure choice at this point, not before |

The build itself costs close to nothing in tooling. The only real cost before revenue is time, which is the right trade for a bootstrapped, one-person product.

## 5. Break-even, rough numbers

At a $7/month average and near-zero infrastructure cost through the free tier and self-hosted fallback, covering a $25/month VPS once one is needed takes roughly 4 paying users. This is intentionally a low bar. The goal in year one is proving people will pay at all, not optimizing margin.

## 6. Metrics that matter before pricing gets locked in

- Free to paid conversion rate
- What free users actually hit the limits of first (this tells you which Pro feature to build next, not the roadmap order)
- Churn on Pro, since a $5 to $10 tool needs to earn its place every month
- Support burden per user, since a solo maintainer's time is the real constraint, not server cost

## 7. Later revenue, not now

A developer marketplace (agents, MCP servers, integrations) is a real future path, through affiliate commissions, sponsored placements clearly labeled as such, and marketplace transaction fees. This is explicitly post-MVP and post-Pro-tier-validation. Building it earlier would be building a second product before the first one has paying users.

## 8. What not to do

- Do not add a paywall before the free tier has proven the core loop works for real users.
- Do not price based on infrastructure cost alone. At this scale, infrastructure is nearly free either way, price based on value to the user instead.
- Do not build billing, subscriptions, or teams infrastructure until there is a waiting list of people asking to pay. That infrastructure has its own maintenance cost and is wasted work if built too early.
