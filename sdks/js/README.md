# ⚡ Flagura JavaScript & TypeScript SDK

Official JavaScript and TypeScript client for the **Flagura Feature Flag Platform**, supporting:
- Sub-millisecond evaluation
- **Real-Time HTTP/2 SSE Flag Streaming (`<5ms` sync)**
- **CNCF OpenFeature Provider**
- React, Next.js, Node.js, and browser support

---

## 📦 Installation

```bash
npm install flagura-sdk
# or
pnpm add flagura-sdk
# or
yarn add flagura-sdk
```

---

## 🚀 Quickstart

### 1. Direct Client with Real-Time SSE Streaming

```typescript
import { FlaguraClient } from 'flagura-sdk';

const client = new FlaguraClient({
  endpoint: 'https://flagura.dev',
  apiKey: process.env.FLAGURA_API_KEY,
  projectId: 'proj_default', // optional: project scoping
  defaultEnvironment: 'production',
  enableStreaming: true, // <5ms live flag updates
});

// Register real-time change listener
const unsubscribe = client.onUpdate((flags) => {
  console.log('Flags updated in real-time!', flags);
});

// Evaluate flag
const isEnabled = await client.isEnabled('ai-smart-search', {
  user_id: 'usr_dhawal_01',
  email: 'dhawal@flagura.dev',
  tier: 'enterprise',
});

if (isEnabled) {
  const variant = await client.getVariant('ai-smart-search', { user_id: 'usr_dhawal_01' });
  console.log('AI Search is ON with variant:', variant);
}

// Cleanup on shutdown
client.close();
```

---

### 2. OpenFeature Universal Provider

```typescript
import { OpenFeature } from '@openfeature/server-sdk';
import { FlaguraOpenFeatureProvider } from 'flagura-sdk';

// Register Flagura as OpenFeature provider
const provider = new FlaguraOpenFeatureProvider({
  endpoint: 'https://flagura.dev',
  apiKey: process.env.FLAGURA_API_KEY,
  enableStreaming: true,
});

await OpenFeature.setProviderAndWait(provider);
const ofClient = OpenFeature.getClient();

// Evaluate with standard OpenFeature APIs
const isEnabled = await ofClient.getBooleanValue('ai-smart-search', false, {
  targetingKey: 'usr_dhawal_01',
  email: 'dhawal@flagura.dev',
});
```

---

## 📄 License
MIT
