import { test, expect } from '@playwright/test';

test.describe('Operator Console E2E Tests (§15)', () => {
  // Test 1: Dry run, then run an experiment, then see verdicts
  test('(1) Dry run, run an experiment, then see verdicts', async ({ page }) => {
    // Navigate to new experiment form
    await page.goto('/replaylab/experiments/new');
    await expect(page.locator('h1')).toHaveText('New experiment');

    // Fill form basics
    const nameInput = page.locator('input[placeholder*="gpt-4o vs"]');
    await nameInput.fill('Playwright E2E Experiment');

    // Select candidate model
    const candidateCheckbox = page.locator('#cand-gpt-4o-mini');
    if (await candidateCheckbox.isVisible()) {
      await candidateCheckbox.click();
    }

    // Verify Estimate Cost button is enabled and click it (Dry Run)
    const estimateBtn = page.locator('button:has-text("Estimate cost")');
    await expect(estimateBtn).toBeVisible();
    await estimateBtn.click();

    // Verify estimate panel updates with dry run results
    await expect(page.locator('text=Estimated cost')).toBeVisible({ timeout: 5000 });
    const runBtn = page.locator('button:has-text("Run for ≈")');
    await expect(runBtn).toBeVisible();

    // Run the experiment
    await runBtn.click();

    // Verify redirection to experiment detail page
    await expect(page).toHaveURL(/.*\/replaylab\/experiments\/exp-.*/, { timeout: 8000 });

    // Inspect completed experiment detail with verdicts
    await page.goto('/replaylab/experiments/exp-29ae0198-6');
    await expect(page.locator('h1')).toHaveText('gpt-4o vs gpt-4o-mini & llama-3.1');

    // Verify candidate verdict panels & comparison table
    await expect(page.locator('text=Safe to switch').first()).toBeVisible();
    await expect(page.locator('text=Do not switch').first()).toBeVisible();
    await expect(page.locator('text=Comparison')).toBeVisible();
    await expect(page.locator('th:has-text("gpt-4o (baseline)")')).toBeVisible();
    await expect(page.locator('th:has-text("gpt-4o-mini")')).toBeVisible();
  });

  // Test 2: Check an answer, then see the report
  test('(2) Check an answer, then see the report', async ({ page }) => {
    await page.goto('/stale/check');
    await expect(page.locator('h1')).toHaveText('Check an answer');

    // Click "Load example" to populate answer
    const loadExampleBtn = page.locator('button:has-text("Load example")');
    await loadExampleBtn.click();

    const textarea = page.locator('textarea');
    await expect(textarea).toContainText('Python 3.9');

    // Trigger check
    const checkBtn = page.locator('button:has-text("Check answer")');
    await checkBtn.click();

    // Verify progress steps become visible/active
    await expect(page.locator('text=Extract claims')).toBeVisible();

    // Wait for report redirection
    await expect(page).toHaveURL(/.*\/stale\/reports\/.*/, { timeout: 15000 });

    // Verify report header
    await expect(page.locator('h1')).toHaveText('Report');

    // Verify original answer panel and highlighted claim marks
    await expect(page.locator('text=Original answer')).toBeVisible();
    const markElements = page.locator('mark');
    await expect(markElements.first()).toBeVisible();

    // Verify claims panel with freshness meter
    await expect(page.locator('.section-title:has-text("Claims")')).toBeVisible();

    // Verify corrected answer panel and tabs
    await expect(page.locator('.section-title:has-text("Corrected answer")')).toBeVisible();
    await expect(page.locator('button[role="tab"]:has-text("Corrected")')).toBeVisible();
    await expect(page.locator('button[role="tab"]:has-text("Changes")')).toBeVisible();
  });

  // Test 3: With Insights down, Overview shows "Analytics offline" and Captures still loads
  test('(3) With Insights down, Overview shows "Analytics offline" and Captures still loads', async ({ page }) => {
    // Intercept insights API calls to simulate service outage
    await page.route('http://localhost:8090/api/**', (route) => {
      route.abort('connectionfailed');
    });

    // Navigate to Overview
    await page.goto('/overview');

    // Verify "Analytics offline" state is displayed
    await expect(page.locator('text=Analytics offline')).toBeVisible({ timeout: 5000 });
    await expect(
      page.locator('text=The insights service at http://localhost:8090 isn\'t reachable. Live features still work.')
    ).toBeVisible();

    // Verify "Go to captures" button is present and navigates to captures
    const goToCapturesBtn = page.locator('button:has-text("Go to captures")');
    await expect(goToCapturesBtn).toBeVisible();
    await goToCapturesBtn.click();

    await expect(page).toHaveURL(/.*\/replaylab\/captures/);
    await expect(page.locator('h1')).toHaveText('Captures');

    // Verify captures still load cleanly
    await expect(page.locator('th:has-text("Model")')).toBeVisible();
    await expect(page.locator('td:has-text("gpt-4o")').first()).toBeVisible({ timeout: 5000 });
  });
});
