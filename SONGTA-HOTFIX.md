# Songta 0.1.172 hotfix

Based on upstream v0.1.172, commit 155c494964c3ea6ecc31f52679525c1034bf0f16.

Responses-to-Chat streaming conversion used to emit function arguments only
from delta events. The observed parallel stream supplied complete arguments in
done events, output-item snapshots and terminal output, leaving Chat callers
with named tools and empty argument strings.

The converter tracks emitted bytes by output_index and sends only a missing
suffix from completion snapshots. Tool indices are stable across repeated item
events. Successful terminal snapshots are sent before finish_reason, and events
after finalization are ignored. Conflicting snapshots are not appended to
already streamed bytes, which cannot be retracted.

Regression tests cover 13 event sequences, including three interleaved calls,
delta/done deduplication, partial deltas, item/terminal fallback, custom tools,
unknown indices, conflicts, duplicate headers and finish ordering. The original
converter fails 10 of these sequences. Run:

    cd backend
    go test -race ./internal/pkg/apicompat -count=1

Dockerfile.songta runs the complete apicompat package before building the
embedded application. Its final image is the exact former production runtime;
only /app/sub2api is replaced. There are no schema, migration, account or volume
changes. The build reports version 0.1.172-songta.1. The Railway source is this
repository's release branch, using railway.toml.

Rollback: reconnect the Sub2API service to the former official image
`ghcr.io/wei-shaw/sub2api:0.1.172@sha256:22eeb8a26a75e92cab72488963605d4cd34edafcd164b34209c7140c9bcf457a`
and redeploy. Keep PostgreSQL, Redis, /app/data and all secrets in place. Remove
the client's serial-tool workaround only after production parallel tool calls
and result round trips have passed.
