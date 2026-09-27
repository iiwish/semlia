import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import test from 'node:test';

const require = createRequire(new URL('../../web/package.json', import.meta.url));
const { JSDOM } = require('jsdom');
const html = await readFile(new URL('../../docs/briefings/2026-09-semantic-platform/system-map.html', import.meta.url), 'utf8');

async function setup(t, width = 1440, height = 900) {
  const viewportSize = { width, height: height - 64 };
  const mapSize = { height: 2500 };
  const observers = [];
  const dom = new JSDOM(html, {
    runScripts: 'dangerously',
    pretendToBeVisual: true,
    beforeParse(window) {
      window.ResizeObserver = class {
        constructor(callback) { this.callback = callback; observers.push(this); }
        observe() {}
      };
      Object.defineProperties(window.HTMLElement.prototype, {
        clientWidth: { get() { return this.id === 'viewport' ? viewportSize.width : 1800; } },
        clientHeight: { get() { return this.id === 'viewport' ? viewportSize.height : 2500; } },
        offsetWidth: { get() { return this.id === 'map' ? 1800 : 300; } },
        offsetHeight: { get() { return this.id === 'map' ? mapSize.height : 120; } },
      });
      window.HTMLElement.prototype.getBoundingClientRect = function () {
        if (this.id === 'viewport') return { left: 0, top: 64, right: viewportSize.width, bottom: viewportSize.height + 64 };
        return { left: 100, top: 150, right: 130, bottom: 180 };
      };
      window.HTMLElement.prototype.setPointerCapture = function (id) { this.captured = id; };
      window.HTMLElement.prototype.hasPointerCapture = function (id) { return this.captured === id; };
      window.HTMLElement.prototype.releasePointerCapture = function () { this.captured = null; };
      window.HTMLDialogElement.prototype.showModal = function () { this.previousFocus = window.document.activeElement; this.open = true; };
      window.HTMLDialogElement.prototype.close = function () { this.open = false; this.previousFocus?.focus(); this.dispatchEvent(new window.Event('close')); };
    },
  });
  t.after(() => dom.window.close());
  const { window } = dom;
  const byId = id => window.document.getElementById(id);
  const flush = () => new Promise(resolve => window.requestAnimationFrame(resolve));
  const state = () => {
    const [, x, y, zoom] = byId('sheet').style.transform.match(/translate3d\(([-\d.]+)px,([-\d.]+)px,0\) scale\(([-\d.]+)\)/);
    return { x: Number(x), y: Number(y), zoom: Number(zoom) };
  };
  const wheel = async options => {
    const event = new window.WheelEvent('wheel', { cancelable: true, clientX: 600, clientY: 400, ...options });
    byId('viewport').dispatchEvent(event);
    assert.equal(event.defaultPrevented, true);
    await flush();
    return state();
  };
  await flush();
  return { window, byId, state, wheel, flush, viewportSize, mapSize, observers };
}

const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 0.001, `${actual} != ${expected}`);

test('five knowledge types own mappings and relationships without a separate binding surface', async t => {
  const { window, byId } = await setup(t);
  const document = window.document;
  assert.equal(document.querySelector('.binding-strip'), null);
  assert.equal(byId('detail-binding'), null);
  assert.equal(document.querySelectorAll('[data-detail]').length, 6);
  assert.match(document.querySelector('.knowledge.asset').textContent, /字段映射/);
  assert.match(document.querySelector('.knowledge.asset').textContent, /键与粒度/);
  assert.doesNotMatch(document.querySelector('.knowledge.asset').textContent, /基础关联|N:1/);
  assert.match(document.querySelector('.knowledge.model').textContent, /统一客户 ID/);
  assert.match(document.querySelector('.knowledge.model').textContent, /场景配置/);
  assert.match(document.querySelector('.route.r2').textContent, /数据资产/);
  assert.match(document.querySelector('.route.r2').textContent, /单资产/);
  assert.match(document.querySelector('.route.r3').textContent, /跨资产缺少模型/);
  assert.match(byId('detail-asset').content.textContent, /客户键唯一/);
  assert.match(byId('detail-model').content.textContent, /关联定义/);
  assert.match(byId('detail-query').content.textContent, /不能走单资产/);
  assert.match(byId('detail-model').content.textContent, /下单时区域/);
  assert.doesNotMatch(html, /独立映射|独立保存、验证发布|五类知识 \+ 映射与关联|映射与关联版本/);
  for (const button of document.querySelectorAll('[data-detail]')) {
    assert.ok(byId(`detail-${button.dataset.detail}`));
  }
});

test('comparison distinguishes Text-to-SQL approaches and labels governance as a target design', async t => {
  const { window, byId } = await setup(t);
  const section = window.document.querySelector('.stage--compare');
  assert.match(byId('compare-heading').textContent, /Text-to-SQL/);
  assert.deepEqual([...section.querySelectorAll('thead th')].map(cell => cell.textContent), [
    '对比点', '上下文增强型 Text-to-SQL', '语义层增强型 Text-to-SQL', '本平台 · 目标设计',
  ]);
  assert.equal(section.querySelectorAll('tbody tr').length, 6);
  for (const row of section.querySelectorAll('tbody tr')) assert.equal(row.children.length, 4);
  assert.match(section.textContent, /不代表其他方案缺少治理能力/);
  assert.match(section.textContent, /不能保证答案正确/);
  assert.match(section.textContent, /RAG 可用于检索/);
  assert.doesNotMatch(section.textContent, /无法对账|修正一次全局生效|依赖提示词约束/);
  assert.equal(section.querySelectorAll('a[href^="https://"]').length, 3);
});

test('removed controls stay removed; initial camera fits desktop and compact desktop width', async t => {
  for (const [width, height] of [[1440, 900], [1024, 768]]) {
    const { byId, state, window } = await setup(t, width, height);
    assert.equal(byId('print'), null);
    assert.equal(window.document.querySelector('[data-view]'), null);
    near(state().zoom, (width - 64) / 1800);
    assert.equal(state().y, 32);
    assert.equal(window.document.querySelectorAll('.knowledge').length, 5);
    assert.ok(window.document.querySelectorAll('.wires > path').length > 15);
  }
});

test('two-axis trackpad panning does not zoom', async t => {
  const { state, wheel, byId, flush } = await setup(t);
  byId('zoom-value').click();
  await flush();
  const before = state();
  const after = await wheel({ deltaX: 60, deltaY: 120 });
  near(after.x, before.x - 60);
  near(after.y, before.y - 120);
  near(after.zoom, before.zoom);
});

test('pinch zoom preserves the point under the pointer and is reversible', async t => {
  const { state, wheel } = await setup(t);
  const before = state();
  const after = await wheel({ deltaY: -25, ctrlKey: true });
  assert.ok(after.zoom > before.zoom);
  near((600 - before.x) / before.zoom, (600 - after.x) / after.zoom);
  near((336 - before.y) / before.zoom, (336 - after.y) / after.zoom);
  const restored = await wheel({ deltaY: 25, ctrlKey: true });
  near(restored.zoom, before.zoom);
  near(restored.x, before.x);
  near(restored.y, before.y);
});

test('line, page, and Shift+wheel deltas use their correct units', async t => {
  const { state, wheel, byId, flush } = await setup(t);
  byId('zoom-value').click();
  await flush();
  const initial = state();
  let current = await wheel({ deltaY: 2, deltaMode: 1 });
  near(current.y, initial.y - 32);
  current = await wheel({ deltaY: 1, deltaMode: 2 });
  near(current.y, initial.y - 32 - 836);
  current = await wheel({ deltaY: 48, shiftKey: true });
  near(current.x, initial.x - 48);
});

test('WebKit gesture scales once, ignores duplicate wheel, and releases normally', async t => {
  const { window, byId, state, wheel, flush } = await setup(t);
  const gesture = (type, scale) => {
    const event = new window.Event(type, { cancelable: true });
    Object.assign(event, { scale, clientX: 600, clientY: 400 });
    byId('viewport').dispatchEvent(event);
    assert.equal(event.defaultPrevented, true);
  };
  const before = state();
  gesture('gesturestart', 1);
  gesture('gesturechange', 1.3);
  await flush();
  near(state().zoom, before.zoom * 1.3);
  const pinched = state();
  await wheel({ deltaY: -40, ctrlKey: true });
  assert.deepEqual(state(), pinched);
  gesture('gestureend', 1.3);
  await wheel({ deltaY: 30 });
  near(state().y, pinched.y - 30);
});

test('zoom limits, reset, fit width, and fit all remain usable', async t => {
  const { byId, state, wheel, flush } = await setup(t);
  for (let i = 0; i < 8; i++) await wheel({ deltaY: -100, ctrlKey: true });
  near(state().zoom, 2);
  assert.equal(byId('zoom-in').disabled, true);
  for (let i = 0; i < 8; i++) await wheel({ deltaY: 100, ctrlKey: true });
  near(state().zoom, 0.2);
  assert.equal(byId('zoom-out').disabled, true);
  byId('zoom-value').click();
  await flush();
  near(state().zoom, 1);
  byId('fit-all').click();
  await flush();
  assert.ok(state().x >= 24 && state().y >= 24);
  assert.ok(2500 * state().zoom <= 836 - 48);
  byId('fit-width').click();
  await flush();
  near(state().zoom, (1440 - 64) / 1800);
});

test('pointer drag releases capture on cancellation and does not hijack detail buttons', async t => {
  const { window, byId, state, flush } = await setup(t);
  byId('zoom-value').click();
  await flush();
  const viewport = byId('viewport');
  const pointer = (target, type, x, y) => {
    const event = new window.MouseEvent(type, { clientX: x, clientY: y, button: 0, bubbles: true, cancelable: true });
    Object.assign(event, { pointerId: 1, isPrimary: true });
    target.dispatchEvent(event);
  };
  const initial = state();
  pointer(viewport, 'pointerdown', 600, 400);
  pointer(viewport, 'pointermove', 650, 450);
  await flush();
  near(state().x, initial.x + 50);
  near(state().y, initial.y + 50);
  pointer(viewport, 'pointercancel', 650, 450);
  assert.equal(viewport.hasPointerCapture(1), false);
  assert.equal(viewport.classList.contains('is-dragging'), false);
  pointer(window.document.querySelector('[data-detail]'), 'pointerdown', 500, 400);
  assert.equal(viewport.hasPointerCapture(1), false);
});

test('keyboard camera controls preserve modifier shortcuts and dialog focus returns', async t => {
  const { window, byId, state, flush } = await setup(t);
  const viewport = byId('viewport');
  const initial = state();
  viewport.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true }));
  await flush();
  near(state().y, initial.y - 72);
  const event = new window.KeyboardEvent('keydown', { key: '+', ctrlKey: true, cancelable: true });
  viewport.dispatchEvent(event);
  assert.equal(event.defaultPrevented, false);
  for (const button of window.document.querySelectorAll('[data-detail]')) {
    button.click();
    assert.equal(byId('detail-dialog').open, true);
    assert.ok(byId('detail-content').textContent.length > 100);
    byId('close-detail').click();
    assert.equal(byId('detail-dialog').open, false);
    assert.equal(window.document.activeElement, button);
  }
});

test('resize keeps a custom view centered and refits an untouched view', async t => {
  const { state, wheel, viewportSize, observers, flush } = await setup(t);
  viewportSize.width = 1024;
  observers[0].callback();
  await flush();
  near(state().zoom, (1024 - 64) / 1800);
  await wheel({ deltaY: 100 });
  const before = state();
  viewportSize.width = 1440;
  observers[0].callback();
  await flush();
  near(state().zoom, before.zoom);
  near(state().x, before.x + 208);
});

test('panning stops at all four edges with only 32px of gutter', async t => {
  for (const [width, height] of [[1440, 900], [1024, 768]]) {
    const { byId, state, wheel, flush, viewportSize } = await setup(t, width, height);
    const fitted = state();
    await wheel({ deltaX: 100000, deltaY: -100000 });
    near(state().x, fitted.x);
    near(state().y, 32);
    byId('zoom-value').click();
    await flush();
    await wheel({ deltaX: -100000, deltaY: -100000 });
    near(state().x, 32);
    near(state().y, 32);
    await wheel({ deltaX: 100000, deltaY: 100000 });
    near(state().x, width - 1800 - 32);
    near(state().y, viewportSize.height - 2500 - 32);
    const edge = state();
    await wheel({ deltaX: -10, deltaY: -10 });
    near(state().x, edge.x + 10);
    near(state().y, edge.y + 10);
  }
});

test('small canvases stay centered through panning and End; zoom and resizing reapply bounds', async t => {
  const { window, byId, state, wheel, flush, viewportSize, observers, mapSize } = await setup(t);
  for (let i = 0; i < 8; i++) await wheel({ deltaY: 100, ctrlKey: true });
  const centered = state();
  near(centered.x, (1440 - 1800 * .2) / 2);
  near(centered.y, (836 - 2500 * .2) / 2);
  await wheel({ deltaX: 100000, deltaY: -100000 });
  assert.deepEqual(state(), centered);
  byId('viewport').dispatchEvent(new window.KeyboardEvent('keydown', { key: 'End' }));
  await flush();
  assert.deepEqual(state(), centered);
  byId('zoom-value').click();
  await flush();
  await wheel({ deltaX: 100000, deltaY: 100000 });
  viewportSize.width = 2000;
  viewportSize.height = 1000;
  observers[0].callback();
  await flush();
  near(state().x, 100);
  near(state().y, 1000 - 2500 - 32);
  mapSize.height = 600;
  observers[1].callback();
  await flush();
  near(state().y, 200);
});

test('pointer and keyboard cannot cross edges; reversing a drag responds immediately', async t => {
  const { window, byId, state, flush } = await setup(t);
  byId('zoom-value').click();
  await flush();
  const viewport = byId('viewport');
  const pointer = (type, x, y) => {
    const event = new window.MouseEvent(type, { clientX: x, clientY: y, button: 0 });
    Object.assign(event, { pointerId: 1, isPrimary: true });
    viewport.dispatchEvent(event);
  };
  pointer('pointerdown', 600, 400);
  pointer('pointermove', 10000, 10000);
  await flush();
  near(state().x, 32);
  near(state().y, 32);
  pointer('pointermove', 9990, 9990);
  await flush();
  near(state().x, 22);
  near(state().y, 22);
  pointer('pointermove', -10000, -10000);
  pointer('pointerup', -10000, -10000);
  await flush();
  near(state().x, 1440 - 1800 - 32);
  near(state().y, 836 - 2500 - 32);
  const edge = state();
  for (const key of ['ArrowRight', 'ArrowDown', 'PageDown', 'End']) {
    viewport.dispatchEvent(new window.KeyboardEvent('keydown', { key }));
    await flush();
    assert.deepEqual(state(), edge);
  }
});
