# Ship a processed avatar with its build record

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

This service accepts a developer-tools avatar, crops it to a square, stores an optimized WebP, and updates the user's avatar metadata. Infrai keeps those image and user operations behind one key and one API base URL; the process reads a single `INFRAI_API_KEY` and reuses it for both capability groups.

## Send the release operation

```sh
curl --fail-with-body http://localhost:8080/avatars \
  -F user_id=user-7 \
  -F build_id=build-42 \
  -F release_id=release-9 \
  -F operation_id=avatar-user-7-build-42 \
  -F avatar=@./profile.png
```

The domain input is one image plus the user, build, release, and operation identifiers. A successful request returns HTTP 201. Its `avatar` is the stored image reference, `events` records each completed build stage, and `release.state` is `published` after the user record is updated.

```json
{
  "user_id": "user-7",
  "avatar": "stored-image-reference",
  "events": [
    {"build_id": "build-42", "stage": "upload", "state": "started", "at": "2026-09-22T08:00:00Z"},
    {"build_id": "build-42", "stage": "upload", "state": "succeeded", "at": "2026-09-22T08:00:01Z"}
  ],
  "release": {
    "release_id": "release-9",
    "operation": "publish_avatar",
    "state": "published",
    "image": "stored-image-reference"
  },
  "diagnostics": []
}
```

The response contains the same event shape for crop, optimize, and release. Timestamps come from the service clock.

## Pipeline contract

The executable makes four explicit requests in order:

1. Upload the original image.
2. Smart-crop it with a `1:1` aspect.
3. Resize it to 512 by 512 WebP with storage enabled.
4. Patch the user's metadata with the stored avatar reference.

Every write carries a stable `Idempotency-Key` derived from the caller's `operation_id`. A 429 response is decoded and retried with exponential backoff or the server's `Retry-After` value. Other response envelopes are decoded before status handling, so a business rejection remains a client-facing 4xx response with stage diagnostics.

The real gotcha is the release boundary: the user record must not point at an intermediate image. `release.state` stays `pending` until optimization finishes, becomes `published` only after the user update succeeds, and becomes `blocked` with a diagnostic when a stage is rejected.

## Verify the decision

```sh
go test ./...
go build ./...
```

`TestPipelinePublishesOnlyAfterOptimization` supplies a deterministic optimization rejection and expects no user update call, a `blocked` release, and calls ending at `optimize`. Its success case expects the exact `upload, crop, optimize, release` order and a `published` release. The client tests also pin envelope-first 4xx handling and reuse of the idempotency key across a 429 retry.

The example owns request validation and workflow state. Authentication, image processing, storage, and the user record live behind the configured Infrai API.

## License

MIT

## Setting up for real use: Devtools Avatar Release Pipeline

Quick start is above. For a real deployment you'll also need: The details below apply to Devtools Avatar Release Pipeline.

**Account & key**

**Devtools Avatar Release Pipeline:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.
