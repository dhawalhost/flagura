import { FlaguraClient } from '../../sdks/js/src/index';
import { FlaguraOpenFeatureProvider } from '../../sdks/js/src/openfeature';
import { OpenFeature } from '@openfeature/server-sdk';

function assert(condition: boolean, msg: string) {
  if (!condition) {
    console.error(`❌ Assertion failed: ${msg}`);
    process.exit(1);
  }
}

async function main() {
  const endpoint = process.env.FLAGURA_ENDPOINT;
  const apiKey = process.env.FLAGURA_API_KEY;

  if (!endpoint || !apiKey) {
    console.error('FLAGURA_ENDPOINT and FLAGURA_API_KEY must be provided');
    process.exit(1);
  }

  console.log(`🧪 Running TypeScript SDK E2E Verification against ${endpoint}...`);

  const client = new FlaguraClient({
    endpoint,
    apiKey,
    defaultEnvironment: 'production',
    enableStreaming: false,
  });

  // 1. Test Boolean Flag
  const boolRes = await client.evaluate('e2e-bool-active', { user_id: 'usr_ts_1' });
  console.log(`  ✓ Boolean Flag (e2e-bool-active): enabled=${boolRes.enabled}, reason=${boolRes.reason}`);
  assert(boolRes.enabled === true, 'e2e-bool-active should be enabled');

  // 2. Test Percentage Rollout Flag (Deterministic hashing)
  const userA = await client.evaluate('e2e-percentage-rollout', { user_id: 'usr_fixed_hash_1' });
  const userB = await client.evaluate('e2e-percentage-rollout', { user_id: 'usr_fixed_hash_1' });
  assert(userA.enabled === userB.enabled, 'Rollout evaluation must be deterministic for the same user_id');
  console.log(`  ✓ Deterministic Rollout (e2e-percentage-rollout): user_fixed_hash_1=${userA.enabled} (consistent)`);

  // 3. Test Rule Targeting Flag
  const devContext = { user_id: 'usr_dev_1', role: 'developer', email: 'dev@flagura.dev' };
  const guestContext = { user_id: 'usr_guest_1', role: 'guest', email: 'guest@flagura.dev' };

  const devRes = await client.evaluate('e2e-rule-targeting', devContext);
  const guestRes = await client.evaluate('e2e-rule-targeting', guestContext);
  console.log(`  ✓ Rule Targeting (e2e-rule-targeting): dev=${devRes.enabled}, guest=${guestRes.enabled}`);
  assert(devRes.enabled === true, 'Developer role should match targeting rule and be enabled');
  assert(guestRes.enabled === false, 'Guest role should not match targeting rule and be disabled');

  // 4. Test Multivariate Flag
  const mvRes = await client.evaluate('e2e-multivariate-models', { user_id: 'usr_ai_model_test' });
  console.log(`  ✓ Multivariate Flag (e2e-multivariate-models): variant=${mvRes.variant}, value=${JSON.stringify(mvRes.value)}`);
  assert(mvRes.variant !== '', 'Multivariate variant should not be empty');

  // 5. CNCF OpenFeature Provider Test
  const provider = new FlaguraOpenFeatureProvider(client);
  await OpenFeature.setProviderAndWait(provider);
  const ofClient = OpenFeature.getClient('e2e-ts-client');

  const ofBool = await ofClient.getBooleanValue('e2e-bool-active', false, { targetingKey: 'usr_ts_1' });
  assert(ofBool === true, 'OpenFeature getBooleanValue should return true for e2e-bool-active');

  const ofDetails = await ofClient.getStringDetails('e2e-multivariate-models', 'default-fallback', { targetingKey: 'usr_ai_model_test' });
  console.log(`  ✓ OpenFeature Provider: boolean=${ofBool}, stringVariant=${ofDetails.variant}`);

  // 6. Telemetry Conversion Ingestion
  await client.track('e2e-bool-active', 'treatment', 'e2e_checkout', 19.99, 'usr_ts_1');
  console.log('  ✓ Telemetry Event: track() executed successfully');

  console.log('🎉 TypeScript SDK E2E Verification PASSED!\n');
}

main().catch((err) => {
  console.error('Fatal TS SDK E2E Error:', err);
  process.exit(1);
});
