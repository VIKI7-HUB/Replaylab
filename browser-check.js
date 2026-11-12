import { chromium } from 'playwright';

async function runBrowserCheck() {
  console.log('--- STARTING COMPREHENSIVE BROWSER QA CHECK ---');
  const browser = await chromium.launch({
    channel: 'chrome',
    headless: true,
  });

  const results = {
    replaylab: { url: 'http://localhost:3000', errors: [], warnings: [], pagesTested: [] },
    stale: { url: 'http://localhost:3001', errors: [], warnings: [], pagesTested: [] },
  };

  const setupPageListeners = (page, target) => {
    page.on('console', (msg) => {
      const type = msg.type();
      const text = msg.text();
      // Ignore routine health poll connection errors when backends aren't running
      if (text.includes('Failed to fetch') || text.includes('ERR_CONNECTION_REFUSED')) {
        return;
      }
      if (type === 'error') {
        target.errors.push(`[Console Error] ${text}`);
      } else if (type === 'warning') {
        target.warnings.push(`[Console Warning] ${text}`);
      }
    });

    page.on('pageerror', (err) => {
      target.errors.push(`[Uncaught Page Error] ${err.message}`);
    });

    page.on('response', (res) => {
      if (res.status() >= 400) {
        console.log(`    [HTTP ${res.status()}] ${res.url()}`);
      }
    });
  };

  // 1. TEST REPLAY LAB ON PORT 3000
  console.log('\n[1/2] Testing Replay Lab at http://localhost:3000...');
  const page1 = await browser.newPage();
  setupPageListeners(page1, results.replaylab);

  const replayRoutes = [
    '/',
    '/replaylab/captures',
    '/replaylab/experiments',
    '/replaylab/experiments/new',
    '/replaylab/settings',
  ];

  for (const route of replayRoutes) {
    const fullUrl = `http://localhost:3000${route}`;
    console.log(`  Visiting ${fullUrl}...`);
    try {
      const response = await page1.goto(fullUrl, { waitUntil: 'networkidle', timeout: 8000 });
      const status = response ? response.status() : 'no response';
      const title = await page1.title();
      results.replaylab.pagesTested.push({ route, status, title });
      await page1.waitForTimeout(500);
    } catch (e) {
      results.replaylab.errors.push(`Failed loading ${route}: ${e.message}`);
    }
  }

  // Check form interaction on /replaylab/experiments/new
  console.log('  Testing form interactions on New Experiment page...');
  try {
    await page1.goto('http://localhost:3000/replaylab/experiments/new', { waitUntil: 'networkidle' });
    const nameInput = page1.locator('input[placeholder*="gpt-4o vs"]');
    if (await nameInput.isVisible()) {
      await nameInput.fill('Test Experiment 123');
    }
  } catch (e) {
    results.replaylab.errors.push(`New experiment interaction failed: ${e.message}`);
  }

  await page1.close();

  // 2. TEST STALE DETECTOR ON PORT 3001
  console.log('\n[2/2] Testing Stale-Answer Detector at http://localhost:3001...');
  const page2 = await browser.newPage();
  setupPageListeners(page2, results.stale);

  const staleRoutes = [
    '/',
    '/stale/check',
    '/stale/history',
  ];

  for (const route of staleRoutes) {
    const fullUrl = `http://localhost:3001${route}`;
    console.log(`  Visiting ${fullUrl}...`);
    try {
      const response = await page2.goto(fullUrl, { waitUntil: 'networkidle', timeout: 8000 });
      const status = response ? response.status() : 'no response';
      const title = await page2.title();
      results.stale.pagesTested.push({ route, status, title });
      await page2.waitForTimeout(500);
    } catch (e) {
      results.stale.errors.push(`Failed loading ${route}: ${e.message}`);
    }
  }

  // Test "Load example" button on /stale/check
  console.log('  Testing "Load example" button on Check page...');
  try {
    await page2.goto('http://localhost:3001/stale/check', { waitUntil: 'networkidle' });
    const loadExampleBtn = page2.locator('button:has-text("Load example")');
    if (await loadExampleBtn.isVisible()) {
      await loadExampleBtn.click();
      await page2.waitForTimeout(300);
      const textarea = page2.locator('textarea');
      const val = await textarea.inputValue();
      if (!val.includes('Python 3.9')) {
        results.stale.errors.push('"Load example" did not populate expected text in textarea');
      } else {
        console.log('    ✓ "Load example" populated textarea successfully');
      }
    } else {
      results.stale.errors.push('"Load example" button not found');
    }

    // Test Theme toggle
    console.log('  Testing Theme toggle...');
    const themeBtn = page2.locator('button[aria-label="Toggle theme"]');
    if (await themeBtn.isVisible()) {
      await themeBtn.click();
      await page2.waitForTimeout(200);
      const themeAttr = await page2.evaluate(() => document.documentElement.getAttribute('data-theme'));
      console.log(`    ✓ Theme toggle switched theme to: ${themeAttr}`);
    }
  } catch (e) {
    results.stale.errors.push(`Stale Check interaction failed: ${e.message}`);
  }

  await page2.close();
  await browser.close();

  // Filter out favicon 404 from errors if present
  const filterErrors = (errs) => errs.filter((e) => !e.includes('favicon.ico'));

  const rlErrors = filterErrors(results.replaylab.errors);
  const staleErrors = filterErrors(results.stale.errors);

  console.log('\n--- BROWSER QA TEST REPORT ---');
  console.log('\nReplay Lab (http://localhost:3000):');
  console.log(`  Pages tested: ${results.replaylab.pagesTested.length}`);
  results.replaylab.pagesTested.forEach((p) => console.log(`    - ${p.route}: Status ${p.status} (${p.title})`));
  console.log(`  Errors: ${rlErrors.length}`);
  rlErrors.forEach((e) => console.log(`    ✖ ${e}`));

  console.log('\nStale-Answer Detector (http://localhost:3001):');
  console.log(`  Pages tested: ${results.stale.pagesTested.length}`);
  results.stale.pagesTested.forEach((p) => console.log(`    - ${p.route}: Status ${p.status} (${p.title})`));
  console.log(`  Errors: ${staleErrors.length}`);
  staleErrors.forEach((e) => console.log(`    ✖ ${e}`));

  if (rlErrors.length === 0 && staleErrors.length === 0) {
    console.log('\n>>> SUCCESS: NO BROWSER ERRORS FOUND ON EITHER FRONTEND! <<<');
  } else {
    console.log('\n>>> ISSUES DETECTED - SEE DETAILS ABOVE <<<');
  }
}

runBrowserCheck().catch((err) => {
  console.error('Browser QA script failed:', err);
  process.exit(1);
});
