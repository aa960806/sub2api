# Canvas protocol replay (2026-09-19)

The three `.js` fixtures are unmodified functions extracted from the public
https://canvas.modelaryx.com client. They execute in a Node VM with only the
necessary local dependencies. `verify.cjs` connects exclusively to the Go test's
127.0.0.1 server. No production account or billable provider request is used.

- `video-module.js`: module 8141 from `/_next/static/chunks/0bsv4mssdje~d.js`.
- `seedance-helpers.js`: module 94299 from that same chunk (includes a harmless shared icon).
- Source chunk SHA256: `a382545a530ee49ea587943f26d1cc1cd8e5bdbcc17a5bfaab5a1514a5bc0b3b`.
- `build-api-url.js`: exported function from `/_next/static/chunks/0~gxp56honmeu.js`.
- Source chunk SHA256: `8150f60a08ba2bd2ab1335c5bae55c7a4c4d7f46121d357c7952172665c58db9`.

Run from `backend` with Node installed:

```sh
go test -tags=unit ./internal/custom/business/media -run TestSeedanceCanvasClientContract -v
```

Coverage: OpenAI model list, Ark task POST without idempotency header, text and
two inline images, durable worker submission, provider upload/account affinity,
settlement, actual client polling and signed URL download without Authorization,
clear rejections for unsupported duration/resolution/video references.
