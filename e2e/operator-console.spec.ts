import { test, expect } from '@playwright/test';

test.describe('LLM Reliability Console', () => {
  // Test 1: Dry run, then run an experiment, then see verdicts
  test('(1) Dry run, run an experiment, then see verdicts', async ({ page }) => {
    // Navigate to new experiment form
    await page.goto('/replaylab/experiments/new');
    await expect(page.locator('h1')).toHaveText('New experiment');

    // Fill form basics (needs gpt-4o captures: run demo/generate_traffic first)
    // The first select on the form is the baseline model
    await page.getByRole('combobox').first().click();
    await page.getByRole('option', { name: 'gpt-4o', exact: true }).click();

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

    // The run finishes and shows a verdict panel and the comparison table
    await expect(page.locator('h1')).toHaveText('Playwright E2E Experiment');
    await expect(page.locator('text=Comparison')).toBeVisible({ timeout: 30000 });
    await expect(page.locator('text=Confidence').first()).toBeVisible();
    await expect(page.locator('th:has-text("(baseline)")')).toBeVisible();
    await expect(page.locator('th:has-text("gpt-4o-mini")')).toBeVisible();
  });

  // Test 3: With Insights down, the console still works
  test('(3) With Insights down, Captures still loads', async ({ page }) => {
    await page.route('http://localhost:8090/api/**', (route) => route.abort('connectionfailed'));

    await page.goto('/');
    await expect(page).toHaveURL(/.*\/replaylab\/captures/);
    await expect(page.locator('h1')).toHaveText('Captures');
    await expect(page.locator('th:has-text("Model")')).toBeVisible();
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

  // Test 4: Both products are pages of the one console
  test('(4) The sidebar moves between the two products without leaving the console', async ({ page }) => {
    await page.goto('/replaylab/captures');
    await page.locator('nav a:has-text("Check")').click();
    await expect(page).toHaveURL(/\/stale\/check$/);
    await expect(page.locator('h1')).toHaveText('Check an answer');
    await page.locator('nav a:has-text("Experiments")').click();
    await expect(page).toHaveURL(/\/replaylab\/experiments$/);
    await expect(page.locator('h1')).toHaveText('Experiments');
  });
});
