# Compute Accounting

COMRAD associates every client request with a user and records compute movement in an append-only ledger.

## API Clients And Keys

Admins create API clients in the dashboard **API clients** section or through `/api/admin/users`. Dashboard operators normally find a client in the table, open **View** to inspect the client detail modal, then edit client status, issue keys, or adjust balance.

Issue an API key:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"userId":"<user-id>","name":"client"}' \
  http://<manager-host>:1922/api/admin/api-keys
```

The response includes the raw API key once. The Manager stores only a hash. Every client request must include:

```sh
-H "Authorization: Bearer <client-api-key>"
```

The key must map to exactly one active API client. Missing, invalid, revoked, or ambiguous keys are rejected. Admins can paste a raw key into the dashboard lookup; the Manager hashes it server-side and returns the matching key metadata and client without exposing `tokenHash` or the raw key.

`COMRAD_CLIENT_API_KEY` is bootstrapped into a default API client for initial local compatibility. Prefer issuing per-client keys for real use.

## Profile Compute Cost

Each Workload Profile has an explicit `computeCost`. If it is missing, the cost is `0`. COMRAD does not infer cost from model size, quantization, runtime, duration, or tokens yet.

Profile YAML:

```yaml
profileId: llm.chat/local/context-4096
model: assistant-default
kind: llm.chat
computeCost: 5
runtime:
  adapter: llama.cpp-metal
  modelArtifacts:
    - sha256:<model>
  contextTokens: 4096
requirements:
  target: darwin-arm64-metal
warmable: true
```

Update cost directly:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"profileId":"llm.chat/local/context-4096","computeCost":5}' \
  http://<manager-host>:1922/api/admin/profiles/compute-cost
```

The selected profile version and effective compute cost are copied onto each Task, Attempt, and Compute Report at execution time.

## Ledger And Balance

The ledger is the source of truth. Cached user balances are updated from ledger appends.

Entry types:

- `consume_compute`: requester spent compute for a successful task.
- `produce_compute`: node owner earned compute for a successful task.
- `admin_adjustment`: admin top-up or correction.
- `purchase_compute`: reserved for future paid purchase flows.

Successful positive-cost tasks create a consumer debit and, when the executing node has an owner, a producer credit. Zero-cost tasks still record the requesting user on task/attempt/report but do not change balances. Failed attempts do not charge by default.

Manual adjustment:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"userId":"<user-id>","amount":10,"reason":"manual top-up"}' \
  http://<manager-host>:1922/api/admin/users/adjust-balance
```

Negative `amount` records a debit correction.

## Balance Enforcement

Set `COMRAD_ENFORCE_BALANCE=true` to require enough balance for positive-cost profiles. Zero-cost profiles remain usable with zero balance.

When enforcement is enabled and balance is insufficient, `/v1/chat/completions` returns `402` with `insufficient_balance`.

Payment processing is not implemented. The data model reserves `purchase_compute` entries so a future paid exchange can credit users without replacing the ledger.
