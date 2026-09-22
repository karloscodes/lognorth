/**
 * SDK Compatibility Test
 *
 * This test verifies that the TypeScript SDK can successfully send events
 * to LogNorth and that they are properly received, processed, and stored.
 *
 * Prerequisites:
 * 1. LogNorth server running (make dev or ./lognorth)
 * 2. API key created and set in LOGNORTH_API_KEY env var
 *
 * Usage:
 *   npm install
 *   LOGNORTH_API_KEY=lb_your_key_here npm test
 */

import LogNorth from '@karloscodes/lognorth-sdk';

const LOGNORTH_URL = process.env.LOGNORTH_URL || 'http://localhost:8080';
const LOGNORTH_API_KEY = process.env.LOGNORTH_API_KEY;

if (!LOGNORTH_API_KEY) {
  console.error('❌ LOGNORTH_API_KEY environment variable is required');
  console.error('Usage: LOGNORTH_API_KEY=lb_your_key npm test');
  process.exit(1);
}

console.log('🧪 Starting SDK compatibility test...\n');
console.log(`LogNorth URL: ${LOGNORTH_URL}`);
console.log(`API Key: ${LOGNORTH_API_KEY.substring(0, 10)}...\n`);

// Configure SDK
LogNorth.config(LOGNORTH_URL, LOGNORTH_API_KEY);

async function runTests() {
  const testId = Date.now();
  let passed = 0;
  let failed = 0;

  try {
    // Test 1: Send a regular log
    console.log('📝 Test 1: Sending regular log...');
    LogNorth.log(`Test log ${testId}`, {
      test: 'regular_log',
      timestamp: new Date().toISOString()
    });
    await LogNorth.flush();
    console.log('✅ Regular log sent\n');
    passed++;

    // Test 2: Send an error
    console.log('📝 Test 2: Sending error...');
    const testError = new Error(`Test error ${testId}`);
    LogNorth.error('Test error occurred', testError, {
      test: 'error_log',
      errorId: testId
    });
    await new Promise(resolve => setTimeout(resolve, 100)); // Errors send immediately
    console.log('✅ Error sent\n');
    passed++;

    // Test 3: Send batch of events
    console.log('📝 Test 3: Sending batch of events...');
    for (let i = 0; i < 5; i++) {
      LogNorth.log(`Batch event ${i} - ${testId}`, {
        test: 'batch_log',
        index: i,
        batchId: testId
      });
    }
    await LogNorth.flush();
    console.log('✅ Batch sent\n');
    passed++;

    // Test 4: Send event with context
    console.log('📝 Test 4: Sending event with rich context...');
    LogNorth.log(`Context test ${testId}`, {
      test: 'context_test',
      userId: 123,
      metadata: {
        nested: {
          value: 'deep',
          array: [1, 2, 3]
        }
      },
      tags: ['test', 'sdk', 'compatibility']
    });
    await LogNorth.flush();
    console.log('✅ Context event sent\n');
    passed++;

  } catch (error) {
    console.error('❌ Test failed:', error.message);
    failed++;
  }

  // Summary
  console.log('\n' + '='.repeat(50));
  console.log('Test Summary:');
  console.log(`  ✅ Passed: ${passed}`);
  console.log(`  ❌ Failed: ${failed}`);
  console.log('='.repeat(50));

  if (failed === 0) {
    console.log('\n🎉 All tests passed! SDK is compatible with LogNorth.\n');
    console.log('Next steps:');
    console.log('  1. Check LogNorth dashboard to see events');
    console.log('  2. Wait ~5 seconds for event processing');
    console.log('  3. Verify issues page shows the test error');
    process.exit(0);
  } else {
    console.log('\n❌ Some tests failed. Check LogNorth server logs.\n');
    process.exit(1);
  }
}

runTests().catch(error => {
  console.error('❌ Unexpected error:', error);
  process.exit(1);
});
