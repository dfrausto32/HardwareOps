# Incident Note: Webhook Test Delivery History Gap

Date:
- March 25, 2026

Environment:
- `https://app.parcel.tryparcel.dev`

Context:
- Live end-to-end validation of the CI feedback-loop additions after deploying the updated control-plane and demo-agent stack to `parcel/dev`.

## Summary

The live smoke test passed `24` checks and failed `1`.

The only remaining failure is that webhook test pings do not appear in webhook delivery history, even though the ping is actually delivered and the HMAC signature is valid.

## Final Test Result

- Passed: `24`
- Failed: `1`
- Skipped: `0`

## Passed Checks

Service-token scope enforcement:
- `scope/create-publish-token`
- `scope/create-read-only-token`
- `scope/publish-token-auth-accepted`
- `scope/read-token-blocked-on-presign`
- `scope/create-trigger-token`
- `scope/create-artifact-read-token`
- `scope/create-webhook-manage-token`
- `scope/artifact-read-token-allowed`
- `scope/webhook-manage-token-allowed`

Webhook flow:
- `webhook/create`
- `webhook/test-ping-queued`
- `webhook/hmac-signature`
- `webhook/redelivery-failed-delivery`
- `webhook/redelivery-created-new-record`
- `webhook/redelivery-success`
- `webhook/cleanup`

Deploy trigger:
- `trigger/device-apply`
- `trigger/scoped-token-accepted`
- `trigger/wrong-scope-rejected`
- `trigger/group-apply`

Deployment status:
- `deploy-status/group-rollout`
- `deploy-status/missing-artifact-param`
- `deploy-status/unknown-group`
- `deploy-status/read-token-allowed`

## Failed Check

- `webhook/delivery-dispatched`

Observed response:

```json
{"items":[]}
```

This response persisted after the smoke test waited 8 seconds for the test-ping delivery record to appear.

## What Was Verified

The failure is narrow and does not indicate a general webhook outage.

Confirmed working:
- webhook creation on live `parcel/dev`
- outbound callback from AWS to a public receiver
- HMAC signature generation and verification
- deploy-trigger event delivery
- failed-delivery recording
- webhook redelivery
- successful redelivery after receiver recovery

That means the external delivery path is functioning.

## Expected Behavior

After:

```text
POST /api/v1/webhooks/{id}/test
```

the resulting synthetic `ping` should create a delivery record visible at:

```text
GET /api/v1/webhooks/{id}/deliveries
```

## Actual Behavior

The synthetic test ping:
- is accepted by the API
- reaches the receiver
- carries a valid HMAC signature

but no corresponding delivery record is visible in webhook delivery history.

## Impact

- Operators can verify connectivity manually, but delivery history for webhook test pings is incomplete.
- The live smoke test cannot fully pass.
- Troubleshooting and auditability are inconsistent between test-ping traffic and normal event-driven deliveries.

## Reproduction

1. Create a webhook on `parcel/dev`.
2. Call `POST /api/v1/webhooks/{id}/test`.
3. Confirm the receiver actually gets the callback.
4. Query `GET /api/v1/webhooks/{id}/deliveries`.
5. Observe that no delivery record exists for the ping test.

## Current Status

- Live backend is deployed and updated.
- Demo fleet is enabled and checking in.
- Trigger and deployment-status flows are working.
- Webhook encryption is configured in the live stack.
- Remaining defect is limited to delivery persistence/visibility for the webhook test-ping path.

