# @emergent-company/runlog-client

TypeScript SDK for the [runlog](https://github.com/emergent-company/runlog) test observability daemon.

## Installation

Published to GitHub Packages (scope `@emergent-company`). Add to `.npmrc`:

```
@emergent-company:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${NODE_AUTH_TOKEN}
```

`NODE_AUTH_TOKEN` needs a GitHub PAT (classic or fine-grained) with `read:packages`.

```sh
npm install @emergent-company/runlog-client
```

## Usage

### Basic client

```typescript
import { RunLogClient } from '@emergent-company/runlog-client'

const client = new RunLogClient('http://localhost:5002')

const { id } = await client.createRun({ testName: 'My test' })
await client.section(id, 'Setup')
await client.log(id, 'Doing work...')
await client.markDone(id, { passed: true })
```

### Jest reporter

Add to `jest.config.js`:

```js
reporters: [
  'default',
  ['@emergent-company/runlog-client/jest', { daemonUrl: 'http://localhost:5002' }],
],
```

Each test file gets a run in the daemon. No changes to existing tests.

### Playwright fixture

```typescript
import { test, expect } from '@emergent-company/runlog-client/playwright'

test('my test', async ({ runlog }) => {
  await runlog.section('Given')
  // ... test steps ...
  await runlog.log('All good')
})
```

The `runlog` fixture is registered with `{ auto: true }` — it runs for every
test with zero body changes (auto-registers the run, captures XHR/fetch
network traffic and uncaught page errors, uploads a screenshot + trace on
completion, and degrades to a no-op if the daemon is unreachable). Destructure
`{ runlog }` only if you want to add manual `.section()`/`.log()` annotations
on top of the automatic capture.

Set `RUNLOG_DAEMON_URL` (default `http://localhost:5002`) to point at a
project-specific daemon instance.

## API

See [`src/types.ts`](./src/types.ts) for all interfaces and [`src/client.ts`](./src/client.ts) for the full client reference.
