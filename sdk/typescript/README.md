# @runlog/client

TypeScript SDK for the [runlog](https://github.com/emergent-company/runlog) test observability daemon.

## Installation

```sh
npm install @runlog/client
```

## Usage

### Basic client

```typescript
import { RunLogClient } from '@runlog/client'

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
  ['@runlog/client/jest', { daemonUrl: 'http://localhost:5002' }],
],
```

Each test file gets a run in the daemon. No changes to existing tests.

### Playwright fixture

```typescript
import { test, expect } from '@runlog/client/playwright'

test('my test', async ({ runlog }) => {
  await runlog.section('Given')
  // ... test steps ...
  await runlog.log('All good')
})
```

## API

See [`src/types.ts`](./src/types.ts) for all interfaces and [`src/client.ts`](./src/client.ts) for the full client reference.
