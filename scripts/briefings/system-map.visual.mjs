import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(new URL('../../web/package.json', import.meta.url));
const { chromium } = require('@playwright/test');
const { default: AxeBuilder } = require('@axe-core/playwright');
const source = new URL('../../docs/briefings/2026-09-semantic-platform/system-map.html', import.meta.url);
const output = fileURLToPath(new URL('../../build/semantic-briefing/system-map-ownership/', import.meta.url));
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true });
const report = [];

try {
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1024, height: 768 }]) {
    const context = await browser.newContext({ viewport, offline: true, reducedMotion: 'reduce' });
    const page = await context.newPage();
    const errors = [], requests = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => { if (/^https?:/.test(request.url())) requests.push(request.url()); });
    await page.goto(source.href);
    await page.evaluate(() => document.fonts.ready);
    const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await settle();
    assert.equal(await page.locator('.knowledge').count(), 5);
    assert.equal(await page.locator('[data-detail]').count(), 6);
    assert.equal(await page.locator('.binding-strip').count(), 0);
    await page.screenshot({ path: `${output}/${viewport.width}-initial.png` });

    const layout = await page.locator('#map').evaluate(map => {
      const issues = [];
      const scale = map.getBoundingClientRect().width / 1800;
      for (const element of map.querySelectorAll('*')) {
        if (!(element instanceof HTMLElement) || !element.clientWidth) continue;
        if (element.scrollWidth > element.clientWidth + 2) issues.push(`overflow: ${element.className || element.tagName}`);
        const card = element.closest('.knowledge, .node, .material');
        if (!card) continue;
        const rect = element.getBoundingClientRect(), bounds = card.getBoundingClientRect();
        if (rect.bottom > bounds.bottom + scale || rect.left < bounds.left - scale || rect.right > bounds.right + scale) {
          issues.push(`outside card: ${card.className} / ${element.className || element.tagName}`);
        }
      }
      const cards = [...map.querySelectorAll('.knowledge, .node, .material')];
      for (let i = 0; i < cards.length; i++) for (let j = i + 1; j < cards.length; j++) {
        const a = cards[i].getBoundingClientRect(), b = cards[j].getBoundingClientRect();
        if (a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top) issues.push('overlapping cards');
      }
      return issues;
    });
    assert.deepEqual(layout, []);

    for (const section of ['knowledge', 'usage', 'compare']) {
      const bounds = await page.locator(`.stage--${section}`).boundingBox();
      await page.mouse.move(viewport.width / 2, viewport.height / 2);
      await page.mouse.wheel(0, bounds.y - 100);
      await settle();
      await page.screenshot({ path: `${output}/${viewport.width}-${section}.png` });
    }
    await page.locator('#fit-all').click();
    await settle();
    await page.screenshot({ path: `${output}/${viewport.width}-overview.png` });
    const accessibility = (await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()).violations;
    assert.deepEqual(accessibility.map(item => item.id), []);

    for (const name of ['object', 'rule', 'metric', 'asset', 'model', 'query']) {
      const trigger = page.locator(`[data-detail="${name}"]`);
      await trigger.focus();
      await page.keyboard.press('Enter');
      assert.equal(await page.locator('#detail-dialog').isVisible(), true);
      assert.equal(await page.locator('#detail-dialog').evaluate(dialog => dialog.scrollWidth <= dialog.clientWidth + 1), true);
      assert.ok((await page.locator('#detail-title').textContent()).length > 0);
      if (['asset', 'model', 'query'].includes(name)) {
        await page.screenshot({ path: `${output}/${viewport.width}-detail-${name}.png` });
        const violations = (await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()).violations;
        assert.deepEqual(violations.map(item => item.id), []);
      }
      await page.keyboard.press('Escape');
      assert.equal(await page.locator('#detail-dialog').isVisible(), false);
      assert.equal(await trigger.evaluate(element => element === document.activeElement), true);
    }

    const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 1, `${actual} != ${expected}`);
    await page.locator('#zoom-value').click();
    await settle();
    await page.mouse.move(8, 100);
    await page.mouse.wheel(-100000, -100000);
    await settle();
    let drawing = await page.locator('#map').boundingBox();
    near(drawing.x, 32);
    near(drawing.y, 64 + 32);
    await page.mouse.down();
    await page.mouse.move(viewport.width - 8, viewport.height - 8, { steps: 5 });
    await settle();
    drawing = await page.locator('#map').boundingBox();
    near(drawing.x, 32);
    near(drawing.y, 64 + 32);
    await page.screenshot({ path: `${output}/${viewport.width}-edge-top-left.png` });
    await page.mouse.move(viewport.width - 28, viewport.height - 28);
    await settle();
    drawing = await page.locator('#map').boundingBox();
    near(drawing.x, 12);
    near(drawing.y, 64 + 12);
    await page.mouse.up();
    await page.mouse.wheel(100000, 100000);
    await settle();
    drawing = await page.locator('#map').boundingBox();
    near(drawing.x + drawing.width, viewport.width - 32);
    near(drawing.y + drawing.height, viewport.height - 32);
    await page.screenshot({ path: `${output}/${viewport.width}-edge-bottom-right.png` });
    while (await page.locator('#zoom-out').isEnabled()) {
      await page.locator('#zoom-out').click();
      await settle();
    }
    await page.mouse.move(8, 100);
    await page.mouse.wheel(100000, -100000);
    await settle();
    drawing = await page.locator('#map').boundingBox();
    near(drawing.x, (viewport.width - drawing.width) / 2);
    near(drawing.y, 64 + (viewport.height - 64 - drawing.height) / 2);
    assert.deepEqual(errors, []);
    assert.deepEqual(requests, []);
    report.push({ viewport, layout, accessibility: [], errors, requests, dialogs: 6, panBoundaries: 'passed' });
    await context.close();
  }
  await writeFile(`${output}/results.json`, JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
} finally {
  await browser.close();
}
