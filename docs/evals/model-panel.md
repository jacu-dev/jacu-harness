# Model panel breaker eval

Unit live-path: a cheap profile is marked down (12 failures). Demotion
keeps the remaining eligible profile, so a later node still has a
candidate. Gemini is not a route target.

| Step | Result |
|---|---|
| ApplyDemotionBias with cheap down | remaining candidate count >= 1 |
| GuardedInvoke missing digest | no process spawned |
| cost.trace | no USD fields |
