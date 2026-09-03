/**
 * Flagura TypeScript Integration Example (Native + OpenFeature)
 *
 * Demonstrates:
 * 1. Direct FlaguraClient evaluation with typed context and streaming sync.
 * 2. Vendor-agnostic CNCF OpenFeature Provider integration.
 */

import { FlaguraClient } from '../../sdks/js/src/index';
import { FlaguraOpenFeatureProvider } from '../../sdks/js/src/openfeature';
import { OpenFeature } from '@openfeature/server-sdk';

async function main() {
  const endpoint = process.env.FLAGURA_ENDPOINT || 'http://localhost:3000';
  const apiKey = process.env.FLAGURA_API_KEY || 'flg_live_demo_key_example';

  console.log('🚀 Flagura TypeScript Integration Example (Native + OpenFeature)');
  console.log(`   Endpoint: ${endpoint}\n`);

  // =========================================================================
  // 1. Native High-Performance Flagura Client
  // =========================================================================
  console.log('--- 1. Native Flagura Client ---');
  const client = new FlaguraClient({
    endpoint,
    apiKey,
    defaultEnvironment: 'production',
    enableStreaming: false, // Set to true for long-running server workers
  });

  const userContext = {
    user_id: 'usr_sarah_99',
    email: 'sarah@company.com',
    country: 'US',
    role: 'developer',
    tier: 'enterprise',
  };

  const evalResult = await client.evaluate('ai-smart-search', userContext);
  console.log(`Flag: ${evalResult.flag_key}`);
  console.log(`Enabled: ${evalResult.enabled}`);
  console.log(`Variant: ${evalResult.variant}`);
  console.log(`Reason: ${evalResult.reason}`);
  console.log(`Bucket: ${evalResult.bucket}%\n`);

  // =========================================================================
  // 2. CNCF OpenFeature Provider Integration
  // =========================================================================
  console.log('--- 2. CNCF OpenFeature Provider ---');

  // Register Flagura as the OpenFeature global provider
  const provider = new FlaguraOpenFeatureProvider(client);
  await OpenFeature.setProviderAndWait(provider);

  // Obtain an OpenFeature standard client
  const ofClient = OpenFeature.getClient('billing-service');

  // OpenFeature standardized evaluation context
  const ofContext = {
    targetingKey: 'usr_sarah_99',
    email: 'sarah@company.com',
    tier: 'enterprise',
  };

  // Evaluate boolean flag
  const isAiSearchActive = await ofClient.getBooleanValue('ai-smart-search', false, ofContext);
  console.log(`OpenFeature getBooleanValue('ai-smart-search'): ${isAiSearchActive}`);

  // Evaluate detailed resolution (with reason and variant)
  const details = await ofClient.getStringDetails('ai-smart-search', 'default-v1', ofContext);
  console.log(`OpenFeature getStringDetails: Value="${details.value}", Variant="${details.variant}", Reason="${details.reason}"`);

  console.log('\n✅ TypeScript Native & OpenFeature evaluations completed successfully.');
}

main().catch((err) => {
  console.error('Error running TypeScript example:', err);
});
