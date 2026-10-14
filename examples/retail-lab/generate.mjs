// Deterministic generator for the Retail Lab synthetic dataset.
// Every name, store, member and transaction is fictional. Money is computed in
// integer cents and rendered as numeric(12,2) yuan so expected answers are exact.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateRawSync } from 'node:zlib';

export const VERSION = 'retail-lab-v1';
export const SEED = 20261014;
export const CUTOFF = '2026-09-30';

const DAY = 86400000;
const TZ_OFFSET = 8 * 3600000; // Asia/Shanghai has no DST.

export const regions = [
  { region_code: 'EC', region_name: '华东', ops_owner: '华东运营部' },
  { region_code: 'SC', region_name: '华南', ops_owner: '华南运营部' },
  { region_code: 'NC', region_name: '华北', ops_owner: '华北运营部' },
  { region_code: 'CC', region_name: '华中', ops_owner: '华中运营部' },
  { region_code: 'SW', region_name: '西南', ops_owner: '西南运营部' },
];
const cities = [
  ['上海', 'EC'], ['杭州', 'EC'], ['南京', 'EC'], ['苏州', 'EC'], ['广州', 'SC'], ['深圳', 'SC'],
  ['北京', 'NC'], ['天津', 'NC'], ['武汉', 'CC'], ['长沙', 'CC'], ['成都', 'SW'], ['重庆', 'SW'],
];
const storeSuffix = ['中心', '滨江', '万象', '湖畔', '新城', '高新', '南站', '大学城'];
export const channels = [
  { channel_code: 'store', channel_name: '门店', is_online: false },
  { channel_code: 'miniapp', channel_name: '小程序', is_online: true },
  { channel_code: 'delivery', channel_name: '外卖平台', is_online: true },
  { channel_code: 'ecommerce', channel_name: '电商平台', is_online: true },
];
const categoryTree = [
  [1, '生鲜', [[11, '水果', ['红富士苹果 1kg', '海南香蕉 1kg', '赣南脐橙 2.5kg', '阳光玫瑰葡萄 500g', '国产蓝莓 125g', '麒麟西瓜 1个', '丹东草莓 300g', '库尔勒香梨 1kg', '泰国金枕榴莲 1个', '佳沛奇异果 6个']],
    [12, '蔬菜', ['有机菠菜 300g', '本地番茄 500g', '黄心土豆 1kg', '西兰花 1颗', '水果黄瓜 500g', '娃娃菜 2颗', '铁棍山药 1kg', '香菇 250g', '紫皮洋葱 1kg', '小米椒 100g']],
    [13, '肉禽蛋', ['冷鲜猪五花 500g', '谷饲牛腩 500g', '散养土鸡蛋 10枚', '白羽鸡全腿 1kg', '内蒙羔羊肉卷 300g', '去骨鸡胸肉 500g', '黑猪梅花肉 400g', '鸭血豆腐 1盒', '牛排套餐 2片', '猪肋排 500g']]]],
  [2, '食品饮料', [[21, '休闲零食', ['每日坚果 25g×7', '海苔脆片 40g', '原味薯片 100g', '芒果干 120g', '牛肉干 80g', '夹心饼干 200g', '黑巧克力 100g', '无糖酸奶块 60g', '花生牛轧糖 150g', '山楂条 200g']],
    [22, '饮料冲调', ['纯牛奶 250ml×12', '无糖乌龙茶 500ml×6', '气泡水 330ml×6', '鲜榨橙汁 1L', '冷萃咖啡 250ml×4', '燕麦奶 1L', '矿泉水 550ml×12', '椰子水 1L', '速溶咖啡 20条', '蜂蜜柚子茶 1kg']],
    [23, '粮油调味', ['东北大米 5kg', '压榨花生油 5L', '特级生抽 500ml', '高筋面粉 2.5kg', '老陈醋 500ml', '挂面 1kg', '赤砂糖 500g', '火锅底料 400g', '小磨香油 200ml', '杂粮组合 2kg']]]],
  [3, '日用百货', [[31, '家庭清洁', ['洗衣凝珠 52颗', '厨房湿巾 80抽', '洗洁精 1.5kg', '地板清洁液 2L', '除菌喷雾 500ml', '垃圾袋 100只', '马桶清洁剂 600g', '洗衣液 3kg', '抹布 5条装', '玻璃清洁剂 500ml']],
    [32, '厨房用品', ['不粘炒锅 30cm', '硅胶铲 2件套', '保鲜盒 3件套', '陶瓷碗 4只', '砧板 1块', '保鲜膜 30m', '筷子 10双', '玻璃水杯 2只', '锡纸 10m', '电子秤 1台']],
    [33, '纸品', ['抽纸 3层×24包', '卷纸 4层×27卷', '湿厕纸 40片×3', '厨房纸 6卷', '手帕纸 36包', '棉柔巾 100抽', '婴儿湿巾 80抽×3', '平板卫生纸 1.2kg', '擦手纸 200抽', '便携纸巾 10包']]]],
  [4, '个护美妆', [[41, '洗护', ['氨基酸洗发水 500ml', '护发素 400ml', '沐浴露 750ml', '洗手液 500ml', '身体乳 400ml', '洗面奶 120g', '护手霜 50g×3', '去屑洗发水 400ml', '儿童沐浴露 300ml', '香皂 4块']],
    [42, '口腔护理', ['含氟牙膏 120g×2', '软毛牙刷 4支', '漱口水 500ml', '牙线棒 100支', '美白牙膏 100g', '电动牙刷头 4支', '儿童牙膏 70g', '冲牙器 1台', '舌苔清洁器 2支', '牙刷旅行套装']],
    [43, '美妆护肤', ['补水面膜 10片', '防晒霜 50ml', '保湿乳液 120ml', '卸妆水 400ml', '精华液 30ml', '润唇膏 2支', '化妆棉 222片', '爽肤水 200ml', '眼霜 15g', '气垫粉底 1盒']]]],
];
const priceRange = { 11: [990, 9990], 12: [290, 1990], 13: [990, 8990], 21: [590, 4990], 22: [690, 6990], 23: [990, 12990],
  31: [990, 5990], 32: [1290, 19990], 33: [990, 6990], 41: [1990, 8990], 42: [890, 29990], 43: [1990, 25990] };
const brands = ['禾田', '安芯', '鲜农坊', '原野', '净朗', '谷丰', '蓝湾', '清源'];
export const promotions = [
  { promo_id: 1, promo_name: '新春年货节', promo_type: 'discount', start_date: '2026-01-20', end_date: '2026-02-10', discount_rate: 12 },
  { promo_id: 2, promo_name: '女神节美妆周', promo_type: 'discount', start_date: '2026-03-05', end_date: '2026-03-10', discount_rate: 15 },
  { promo_id: 3, promo_name: '五一生鲜季', promo_type: 'discount', start_date: '2026-04-28', end_date: '2026-05-05', discount_rate: 10 },
  { promo_id: 4, promo_name: '618年中大促', promo_type: 'discount', start_date: '2026-06-01', end_date: '2026-06-18', discount_rate: 18 },
  { promo_id: 5, promo_name: '开学季囤货', promo_type: 'discount', start_date: '2026-08-25', end_date: '2026-09-05', discount_rate: 10 },
  { promo_id: 6, promo_name: '中秋团圆礼', promo_type: 'discount', start_date: '2026-09-18', end_date: '2026-09-27', discount_rate: 12 },
];
const refundReasons = ['quality', 'wrong_item', 'no_longer_needed', 'late_delivery', 'other'];

const ymd = ms => new Date(ms).toISOString().slice(0, 10);
const day = text => Date.parse(text + 'T00:00:00Z');
// Local (Shanghai) wall-clock milliseconds -> timestamptz literal.
const stamp = local => new Date(local).toISOString().replace('T', ' ').slice(0, 19) + '+08';
const localDate = local => ymd(local);

export function generate(seed = SEED) {
  let state = seed >>> 0;
  const random = max => { state = (Math.imul(state, 1664525) + 1013904223) >>> 0; return Math.floor((state / 0x100000000) * max); };
  const chance = p => random(1_000_000) < p * 1_000_000;
  const weighted = entries => { let r = random(entries.reduce((n, [, w]) => n + w, 0)); for (const [v, w] of entries) { if ((r -= w) < 0) return v; } return entries.at(-1)[0]; };

  const product_categories = [], products = [];
  for (const [parentId, parentName, leaves] of categoryTree) {
    product_categories.push({ category_id: parentId, parent_id: null, category_name: parentName, level: 1 });
    for (const [leafId, leafName, names] of leaves) {
      product_categories.push({ category_id: leafId, parent_id: parentId, category_name: leafName, level: 2 });
      for (const name of names) {
        const [low, high] = priceRange[leafId];
        const list = Math.round((low + random(high - low)) / 10) * 10;
        const privateLabel = chance(0.3);
        products.push({ sku_id: 'SKU' + String(products.length + 1).padStart(4, '0'), sku_name: name, category_id: leafId,
          brand: privateLabel ? '星禾优选' : brands[random(brands.length)], is_private_label: privateLabel,
          list_price: list, cost_price: Math.round(list * (0.52 + random(26) / 100)), launched_on: ymd(day('2024-01-01') + random(700) * DAY), status: 'active' });
      }
    }
  }
  products[17].status = 'discontinued';
  products[63].status = 'discontinued';

  const stores = [];
  cities.forEach(([city, region], cityIndex) => {
    for (let k = 0; k < 2; k++) {
      const n = stores.length + 1;
      stores.push({ store_id: 'S' + String(n).padStart(3, '0'), store_name: `${city}${storeSuffix[(cityIndex + k * 3) % storeSuffix.length]}店`, city,
        region_code: region, store_type: n % 3 === 0 ? 'franchise' : 'direct', area_sqm: 120 + random(260),
        opened_on: ymd(day('2023-03-01') + random(800) * DAY), closed_on: null });
    }
  });
  // Lifecycle edge cases referenced by the store management document.
  stores[5].opened_on = '2026-03-15';  // S006 南京新店：不进入同店口径
  stores[13].opened_on = '2026-05-01'; // S014 北京新店
  stores[21].closed_on = '2026-06-30'; // S022 成都闭店
  // 长沙门店 2026-07-01 起从华南划入华中；门店主数据只保存当前归属。
  const store_region_history = [];
  for (const store of stores) {
    if (store.city === '长沙') {
      store_region_history.push({ store_id: store.store_id, region_code: 'SC', valid_from: store.opened_on, valid_to: '2026-06-30' });
      store_region_history.push({ store_id: store.store_id, region_code: 'CC', valid_from: '2026-07-01', valid_to: null });
    } else store_region_history.push({ store_id: store.store_id, region_code: store.region_code, valid_from: store.opened_on, valid_to: store.closed_on });
  }

  const members = [];
  for (let i = 0; i < 3200; i++) {
    const registered = day('2024-01-01') + random(1003) * DAY + random(86400) * 1000;
    const home = stores[random(stores.length)];
    members.push({ member_id: 100001 + i, registered_at: stamp(registered), register_channel: weighted([['store', 50], ['miniapp', 40], ['ecommerce', 10]]),
      gender: weighted([['F', 58], ['M', 38], ['U', 4]]), birth_year: 1960 + random(45), city: chance(0.05) ? null : home.city, home_store_id: home.store_id,
      tier: weighted([['normal', 60], ['silver', 25], ['gold', 12], ['black', 3]]), is_employee: chance(0.015), _registered: registered });
  }
  members.sort((a, b) => a._registered - b._registered);
  members.forEach((m, i) => { m.member_id = 100001 + i; });

  const orders = [], order_items = [], refunds = [];
  const cutoffEnd = day(CUTOFF) + DAY - 1000;
  const fresh = new Set([11, 12, 13]);
  for (let d = day('2026-01-01'); d <= day(CUTOFF); d += DAY) {
    const date = ymd(d);
    const month = Number(date.slice(5, 7));
    const weekday = new Date(d).getUTCDay();
    const promo = promotions.find(p => date >= p.start_date && date <= p.end_date);
    let count = 22 + month * 1.6 + random(10);
    if (weekday === 0 || weekday === 6) count *= 1.3;
    if (promo) count *= 1.7;
    const openStores = stores.filter(s => s.opened_on <= date && (!s.closed_on || date <= s.closed_on));
    const registeredMembers = members.filter(m => m._registered < d);
    for (let k = 0; k < Math.round(count); k++) {
      const channel = weighted([['store', 55], ['miniapp', 20], ['delivery', 17], ['ecommerce', 8]]);
      const isTest = chance(0.008);
      const member = !isTest && chance(0.68) && registeredMembers.length ? registeredMembers[random(registeredMembers.length)] : null;
      const hour = channel === 'delivery' ? weighted([[11, 3], [12, 4], [17, 3], [18, 4], [19, 3], [20, 2], [21, 2], [22, 2], [23, 2]]) : 8 + random(15);
      const ordered = d + hour * 3600000 + random(3600) * 1000;
      const order = { order_id: orders.length + 1, order_no: '', member_id: member?.member_id ?? null,
        store_id: channel === 'ecommerce' ? null : openStores[random(openStores.length)].store_id, channel_code: channel,
        order_type: isTest ? 'test' : member?.is_employee ? 'staff' : 'normal', promo_id: null, status: 'completed',
        ordered_at: stamp(ordered), paid_at: null, goods_amount: 0, discount_amount: 0, coupon_amount: 0, shipping_fee: 0, paid_amount: 0,
        _ordered: ordered, _paid: null };
      order.order_no = `XH${date.replaceAll('-', '')}${String(order.order_id).padStart(6, '0')}`;
      const lines = [];
      const lineCount = 1 + weighted([[0, 30], [1, 30], [2, 20], [3, 12], [4, 8]]);
      const used = new Set();
      while (lines.length < lineCount) {
        let product = products[random(products.length)];
        if (channel === 'store' && !fresh.has(product.category_id) && chance(0.35)) product = products[random(30)];
        if (used.has(product.sku_id) || product.status === 'discontinued') continue;
        used.add(product.sku_id);
        const quantity = 1 + weighted([[0, 60], [1, 25], [2, 10], [3, 5]]);
        lines.push({ product, quantity, gross: quantity * product.list_price });
      }
      order.goods_amount = lines.reduce((n, l) => n + l.gross, 0);
      if (promo && !isTest && chance(0.6)) {
        order.promo_id = promo.promo_id;
        order.discount_amount = Math.floor(order.goods_amount * promo.discount_rate / 100);
      }
      let allocated = 0;
      lines.forEach((line, index) => {
        const share = index === lines.length - 1 ? order.discount_amount - allocated : Math.floor(order.discount_amount * line.gross / order.goods_amount);
        allocated += share;
        order_items.push({ order_id: order.order_id, line_no: index + 1, sku_id: line.product.sku_id, quantity: line.quantity, unit_price: line.product.list_price,
          discount_amount: share, line_amount: line.gross - share, unit_cost: line.product.cost_price });
      });
      const net = order.goods_amount - order.discount_amount;
      if (member && chance(0.15)) { const coupon = [500, 1000, 2000][random(3)]; if (coupon < net - 100) order.coupon_amount = coupon; }
      order.shipping_fee = channel === 'delivery' ? 600 : channel === 'ecommerce' && order.goods_amount < 9900 ? 800 : 0;
      order.paid_amount = net - order.coupon_amount + order.shipping_fee;
      const outcome = weighted([['paid', 880], ['cancelled', 50], ['closed', 70]]);
      if (outcome === 'paid') {
        order._paid = ordered + (10 + random(1200)) * 1000;
        if (order._paid > cutoffEnd) order._paid = null;
      }
      if (!order._paid) order.status = outcome === 'paid' ? 'pending_payment' : outcome;
      else {
        order.paid_at = stamp(order._paid);
        const age = cutoffEnd - order._paid;
        if (channel !== 'store' && age < 2 * DAY) order.status = age < DAY ? 'paid' : 'shipped';
      }
      if (date === CUTOFF && outcome === 'paid' && chance(0.15)) { order.status = 'pending_payment'; order.paid_at = null; order._paid = null; }
      orders.push(order);

      if (!order._paid) continue;
      const myItems = order_items.slice(-lines.length);
      const goodsPaid = order.paid_amount - order.shipping_fee;
      const addRefund = (amount, shipping, applied, lineNo, forceSucceeded) => {
        let status = forceSucceeded ? 'succeeded' : weighted([['succeeded', 82], ['pending', 10], ['rejected', 8]]);
        let completed = status === 'pending' ? null : applied + (2 + random(72)) * 3600000;
        if (completed && completed > cutoffEnd) { status = 'pending'; completed = null; }
        if (applied > cutoffEnd) return;
        refunds.push({ refund_id: refunds.length + 1, order_id: order.order_id, line_no: lineNo, applied_at: stamp(applied), completed_at: completed ? stamp(completed) : null,
          status, refund_amount: amount, shipping_refund: shipping, reason_code: refundReasons[random(refundReasons.length)] });
      };
      if (order.order_type !== 'test' && chance(0.015) && order.status !== 'paid') {
        // 支付后取消：整单退款（含运费），订单状态记为 cancelled，但 paid_at 保留。
        order.status = 'cancelled';
        addRefund(order.paid_amount, order.shipping_fee, order._paid + (600 + random(7200)) * 1000, null, true);
      } else if (chance(0.055)) {
        const victims = chance(0.08) && myItems.length > 1 ? myItems.slice(0, 2) : [myItems[random(myItems.length)]];
        let remaining = goodsPaid;
        for (const item of victims) {
          const amount = Math.min(item.line_amount, remaining);
          if (amount <= 0) break;
          remaining -= amount;
          addRefund(amount, 0, order._paid + (3600 + random(10 * 86400)) * 1000, item.line_no, false);
        }
      }
    }
  }

  // Month-end semi-additive inventory for one third of the SKUs.
  const inventory_snapshots = [];
  for (let m = 1; m <= 9; m++) {
    const end = ymd(Date.UTC(2026, m, 0));
    for (const store of stores) {
      if (store.opened_on > end || (store.closed_on && store.closed_on < end)) continue;
      products.forEach((product, index) => {
        if (index % 3 !== 0) return;
        inventory_snapshots.push({ snapshot_date: end, store_id: store.store_id, sku_id: product.sku_id,
          on_hand_qty: fresh.has(product.category_id) ? random(60) : 10 + random(190) });
      });
    }
  }

  // Store targets: the store's own valid net sales in that month times a planning factor.
  const valid = orders.filter(o => o._paid && o.order_type !== 'test' && o.status !== 'cancelled');
  const goodsRefund = new Map();
  for (const r of refunds) if (r.status === 'succeeded') goodsRefund.set(r.order_id, (goodsRefund.get(r.order_id) ?? 0) + r.refund_amount - r.shipping_refund);
  const netOf = o => o.paid_amount - o.shipping_fee - (goodsRefund.get(o.order_id) ?? 0);
  const storeMonth = new Map();
  for (const o of valid) if (o.store_id) { const key = o.store_id + localDate(o._paid).slice(0, 7); storeMonth.set(key, (storeMonth.get(key) ?? 0) + netOf(o)); }
  const sales_targets = [];
  for (let m = 1; m <= 9; m++) {
    const month = `2026-${String(m).padStart(2, '0')}`;
    for (const store of stores) {
      const actual = storeMonth.get(store.store_id + month);
      if (!actual) continue;
      sales_targets.push({ store_id: store.store_id, target_month: month + '-01', target_amount: Math.max(100000, Math.round(actual * (85 + random(35)) / 100 / 100000) * 100000) });
    }
  }

  // Deprecated daily report: UTC business date, includes test orders, ignores refunds.
  const legacyMap = new Map();
  for (const o of orders) {
    if (!o._paid || !o.store_id) continue;
    const key = ymd(o._paid - TZ_OFFSET) + '|' + o.store_id;
    const row = legacyMap.get(key) ?? { biz_date: key.slice(0, 10), store_id: o.store_id, order_cnt: 0, sales_amt: 0 };
    row.order_cnt++; row.sales_amt += o.paid_amount; legacyMap.set(key, row);
  }
  const rpt_daily_sales = [...legacyMap.values()].sort((a, b) => a.biz_date.localeCompare(b.biz_date) || a.store_id.localeCompare(b.store_id));

  return { version: VERSION, seed, regions, stores, store_region_history, channels, product_categories, products, members, promotions,
    orders, order_items, refunds, inventory_snapshots, sales_targets, rpt_daily_sales };
}

// ---------------------------------------------------------------------------
// Expected answers, computed independently of SQL from the in-memory data.
const money = cents => { const sign = cents < 0 ? '-' : ''; const v = Math.abs(cents); return `${sign}${Math.floor(v / 100)}.${String(v % 100).padStart(2, '0')}`; };
// Round a/b to `places` decimals, half away from zero (PostgreSQL numeric round).
const ratio = (a, b, places) => { const scale = 10n ** BigInt(places); const n = BigInt(a) * scale * 2n + BigInt(b); const q = n / (2n * BigInt(b)); const s = q.toString().padStart(places + 1, '0'); return `${s.slice(0, -places)}.${s.slice(-places)}`; };

export function expectedAnswers(data) {
  const valid = data.orders.filter(o => o._paid && o.order_type !== 'test' && o.status !== 'cancelled');
  const month = o => localDate(o._paid).slice(0, 7);
  const goodsRefund = new Map();
  for (const r of data.refunds) if (r.status === 'succeeded') goodsRefund.set(r.order_id, (goodsRefund.get(r.order_id) ?? 0) + r.refund_amount - r.shipping_refund);
  const net = o => o.paid_amount - o.shipping_fee - (goodsRefund.get(o.order_id) ?? 0);
  const storeById = new Map(data.stores.map(s => [s.store_id, s]));
  const sum = (rows, f) => rows.reduce((n, x) => n + f(x), 0);
  const aug = valid.filter(o => month(o) === '2026-08');
  const regionName = new Map(data.regions.map(r => [r.region_code, r.region_name]));

  const byRegion = new Map();
  for (const o of aug) if (o.store_id) { const r = storeById.get(o.store_id).region_code; byRegion.set(r, (byRegion.get(r) ?? 0) + net(o)); }
  const regionRows = [...byRegion].sort((a, b) => a[0].localeCompare(b[0])).map(([code, v]) => [regionName.get(code), money(v)]);

  const q2 = ['2026-04', '2026-05', '2026-06'].map(m => [m, String(valid.filter(o => month(o) === m).length)]);

  const augRefund = sum(aug, o => goodsRefund.get(o.order_id) ?? 0);
  const augGoodsPaid = sum(aug, o => o.paid_amount - o.shipping_fee);

  const categoryParent = new Map(data.product_categories.map(c => [c.category_id, c.parent_id ?? c.category_id]));
  const categoryName = new Map(data.product_categories.map(c => [c.category_id, c.category_name]));
  const skuCategory = new Map(data.products.map(p => [p.sku_id, categoryParent.get(p.category_id)]));
  const augIds = new Set(aug.map(o => o.order_id));
  const profit = new Map();
  for (const item of data.order_items) if (augIds.has(item.order_id)) { const c = skuCategory.get(item.sku_id); profit.set(c, (profit.get(c) ?? 0) + item.line_amount - item.quantity * item.unit_cost); }
  const profitRows = [...profit].sort((a, b) => a[0] - b[0]).map(([c, v]) => [categoryName.get(c), money(v)]);

  const memberOrders = valid.filter(o => o.member_id && o.order_type === 'normal');
  const first = new Map();
  for (const o of memberOrders) if (!first.has(o.member_id) || o._paid < first.get(o.member_id)) first.set(o.member_id, o._paid);
  const newMembers = [...first.values()].filter(t => localDate(t).slice(0, 7) === '2026-08').length;

  const window = memberOrders.filter(o => ['2026-07', '2026-08'].includes(month(o)));
  const perMember = new Map();
  for (const o of window) perMember.set(o.member_id, (perMember.get(o.member_id) ?? 0) + 1);
  const repeaters = [...perMember.values()].filter(n => n >= 2).length;

  const target = new Map(), actual = new Map();
  for (const t of data.sales_targets) if (t.target_month === '2026-08-01') { const r = storeById.get(t.store_id).region_code; target.set(r, (target.get(r) ?? 0) + t.target_amount); }
  for (const o of aug) if (o.store_id) { const r = storeById.get(o.store_id).region_code; actual.set(r, (actual.get(r) ?? 0) + net(o)); }
  const achievement = [...target.keys()].sort().map(r => [regionName.get(r), ratio(actual.get(r) ?? 0, target.get(r), 4)]);

  const ecStores = new Set(data.stores.filter(s => s.region_code === 'EC').map(s => s.store_id));
  const inventory = sum(data.inventory_snapshots.filter(s => s.snapshot_date === '2026-08-31' && ecStores.has(s.store_id)), s => s.on_hand_qty);

  const changsha = new Set(data.stores.filter(s => s.city === '长沙').map(s => s.store_id));
  const historyNet = (code, m) => sum(valid.filter(o => month(o) === m && o.store_id && (changsha.has(o.store_id) ? (m <= '2026-06' ? 'SC' : 'CC') : storeById.get(o.store_id).region_code) === code), net);

  return {
    net_sales_2026_08: [[money(sum(aug, net))]],
    gmv_2026_08: [[money(sum(aug, o => o.goods_amount))]],
    paid_orders_2026_08: [[String(aug.length)]],
    aov_2026_08: [[ratio(sum(aug, net), aug.length * 100, 2)]],
    net_sales_by_region_2026_08: regionRows,
    paid_orders_by_month_2026_q2: q2,
    refund_rate_2026_08: [[ratio(augRefund, augGoodsPaid, 4)]],
    gross_profit_by_category_2026_08: profitRows,
    new_members_2026_08: [[String(newMembers)]],
    repurchase_rate_2026_07_08: [[ratio(repeaters, perMember.size, 4)]],
    target_achievement_by_region_2026_08: achievement,
    ec_inventory_2026_08_31: [[String(inventory)]],
    south_china_net_sales_2026_06_historical: [[money(historyNet('SC', '2026-06'))]],
    ecommerce_net_sales_2026_08: [[money(sum(aug.filter(o => o.channel_code === 'ecommerce'), net))]],
  };
}

// ---------------------------------------------------------------------------
// Renderers
const sqlLiteral = value => value === null || value === undefined ? 'NULL' : typeof value === 'boolean' ? (value ? 'true' : 'false')
  : typeof value === 'number' ? String(value) : `'${String(value).replaceAll("'", "''")}'`;
const centsColumns = new Set(['list_price', 'cost_price', 'goods_amount', 'discount_amount', 'coupon_amount', 'shipping_fee', 'paid_amount', 'unit_price',
  'line_amount', 'unit_cost', 'refund_amount', 'shipping_refund', 'target_amount', 'sales_amt']);
const publicColumns = row => Object.keys(row).filter(k => !k.startsWith('_'));
const cell = (key, value) => centsColumns.has(key) && typeof value === 'number' ? money(value) : value;

export const tables = [
  ['retail', 'regions'], ['retail', 'stores'], ['retail', 'store_region_history'], ['retail', 'channels'], ['retail', 'product_categories'],
  ['retail', 'products'], ['retail', 'members'], ['retail', 'promotions'], ['retail', 'orders'], ['retail', 'order_items'], ['retail', 'refunds'],
  ['retail', 'inventory_snapshots'], ['retail', 'sales_targets'], ['legacy', 'rpt_daily_sales'],
];

export function renderSeed(data) {
  const out = [`-- Retail Lab synthetic data (${data.version}, seed ${data.seed}). Generated by generate.mjs; do not edit.\n-- Every person, store and transaction is fictional. Amounts are CNY yuan.\nBEGIN;\nSET LOCAL client_min_messages = warning;\n`];
  for (const [schema, table] of tables) {
    const rows = data[table];
    const columns = publicColumns(rows[0]);
    for (let i = 0; i < rows.length; i += 1000) {
      out.push(`INSERT INTO ${schema}.${table} (${columns.join(', ')}) VALUES\n`
        + rows.slice(i, i + 1000).map(row => '(' + columns.map(c => sqlLiteral(cell(c, row[c]))).join(', ') + ')').join(',\n') + ';\n');
    }
  }
  out.push("SELECT setval(pg_get_serial_sequence('retail.orders', 'order_id'), (SELECT max(order_id) FROM retail.orders));\n");
  out.push("SELECT setval(pg_get_serial_sequence('retail.refunds', 'refund_id'), (SELECT max(refund_id) FROM retail.refunds));\n");
  out.push('COMMIT;\n');
  return out.join('');
}

export function renderCSV(header, rows) {
  const quote = v => { const s = v === null || v === undefined ? '' : String(v); return /[",\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s; };
  return [header, ...rows].map(r => r.map(quote).join(',')).join('\n') + '\n';
}

// Minimal OOXML workbook with inline strings only (no formulas, no shared strings).
export function renderXLSX(sheets) {
  const esc = s => String(s).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
  const col = n => { let s = ''; for (n++; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + ((n - 1) % 26)) + s; return s; };
  const files = [];
  const add = (name, text) => files.push([name, Buffer.from(text, 'utf8')]);
  add('[Content_Types].xml', `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>${sheets.map((_, i) => `<Override PartName="/xl/worksheets/sheet${i + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`).join('')}</Types>`);
  add('_rels/.rels', `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`);
  add('xl/workbook.xml', `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>${sheets.map((s, i) => `<sheet name="${esc(s.name)}" sheetId="${i + 1}" r:id="rId${i + 1}"/>`).join('')}</sheets></workbook>`);
  add('xl/_rels/workbook.xml.rels', `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${sheets.map((_, i) => `<Relationship Id="rId${i + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${i + 1}.xml"/>`).join('')}</Relationships>`);
  sheets.forEach((sheet, i) => {
    const rows = [sheet.header, ...sheet.rows].map((row, r) => `<row r="${r + 1}">${row.map((v, c) => v === null || v === '' ? '' : typeof v === 'number'
      ? `<c r="${col(c)}${r + 1}"><v>${v}</v></c>` : `<c r="${col(c)}${r + 1}" t="inlineStr"><is><t xml:space="preserve">${esc(v)}</t></is></c>`).join('')}</row>`).join('');
    add(`xl/worksheets/sheet${i + 1}.xml`, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${rows}</sheetData></worksheet>`);
  });
  return zip(files);
}

const crcTable = Array.from({ length: 256 }, (_, n) => { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; return c >>> 0; });
const crc32 = buffer => { let c = 0xffffffff; for (const b of buffer) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0; };
function zip(files) {
  const locals = [], centrals = [];
  let offset = 0;
  for (const [name, data] of files) {
    const nameBuffer = Buffer.from(name, 'utf8');
    const compressed = deflateRawSync(data);
    const crc = crc32(data);
    const header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50, 0); header.writeUInt16LE(20, 4); header.writeUInt16LE(0x0800, 6); header.writeUInt16LE(8, 8);
    header.writeUInt32LE(0x00210000, 10); // 2016-01-01 00:00 DOS timestamp, fixed for reproducibility
    header.writeUInt32LE(crc, 14); header.writeUInt32LE(compressed.length, 18); header.writeUInt32LE(data.length, 22); header.writeUInt16LE(nameBuffer.length, 26);
    locals.push(header, nameBuffer, compressed);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6); central.writeUInt16LE(0x0800, 8); central.writeUInt16LE(8, 10);
    central.writeUInt32LE(0x00210000, 12); central.writeUInt32LE(crc, 16); central.writeUInt32LE(compressed.length, 20); central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(nameBuffer.length, 28); central.writeUInt32LE(offset, 42);
    centrals.push(central, nameBuffer);
    offset += 30 + nameBuffer.length + compressed.length;
  }
  const centralBuffer = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(files.length, 8); end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(centralBuffer.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, centralBuffer, end]);
}

// ---------------------------------------------------------------------------
// Spreadsheet-style knowledge files and the dbt project export.
function knowledgeFiles(data) {
  const regionName = new Map(data.regions.map(r => [r.region_code, r.region_name]));
  const storeRows = data.stores.map(s => [s.store_id, s.store_name, s.city, regionName.get(s.region_code), s.store_type === 'direct' ? '直营' : '加盟', s.area_sqm, s.opened_on, s.closed_on ?? '']);
  const targetRows = data.sales_targets.map(t => [t.store_id, t.target_month.slice(0, 7), money(t.target_amount)]);
  const metricRows = [
    ['M01', 'GMV', '成交额;流水;销售流水', '有效支付订单的商品原价金额合计（优惠前，不含运费，不扣退款）', 'sum(goods_amount)', '运营', '已确认'],
    ['M02', '净销售额', '实收;净收入;销售额;营收', '有效支付订单实付金额减运费，再减去这些订单已成功退款的商品金额', 'sum(paid_amount-shipping_fee)-sum(成功退款商品金额)', '财务', '已确认'],
    ['M03', '支付订单数', '单量;订单量;成交单数', '有效支付订单的数量', 'count(distinct order_id)', '运营', '已确认'],
    ['M04', '客单价', '平均订单金额;AOV;笔单价', '净销售额除以支付订单数', '净销售额/支付订单数', '财务', '已确认'],
    ['M05', '退款率', '退货率', '成功退款商品金额除以实付商品金额（按订单支付月归属）', '成功退款商品金额/sum(paid_amount-shipping_fee)', '客服', '已确认'],
    ['M06', '商品毛利', '毛利;毛利额', '订单行实收金额减去行成本，不分摊优惠券、不扣退款', 'sum(line_amount-quantity*unit_cost)', '财务', '已确认'],
    ['M07', '新会员首单数', '新客数;新客', '首个有效支付普通订单落在统计期内的会员数', 'count(distinct member_id)', '会员中心', '已确认'],
    ['M08', '复购率', '回购率;回头客占比', '统计期内有效支付普通订单数不少于2的会员占有购买会员的比例', '复购会员数/购买会员数', '会员中心', '已确认'],
    ['M09', '销售目标达成率', '达成率;完成率', '门店归属净销售额除以门店月度目标', '净销售额/目标金额', '运营', '已确认'],
    ['M10', '期末库存量', '库存;在库量', '月末快照的在库数量，只能在同一快照日期内跨门店或商品求和', 'sum(on_hand_qty) where snapshot_date=月末', '供应链', '已确认'],
    ['M11', '同店销售额', '可比门店销售额;LFL', '统计期与去年同期均完整营业的门店净销售额', '待定', '运营', '待确认'],
    ['M12', '活跃会员数', '活跃用户;月活', '近90天内有有效支付订单的会员数（是否包含员工待定）', 'count(distinct member_id)', '会员中心', '待确认'],
  ];
  const glossary = [
    ['GMV', 'GMV;成交额;流水', '指标', '运营常说的"流水"是 GMV，不是净销售额', '净销售额'],
    ['净销售额', '实收;净收入;销售额', '指标', '财务报表口径；口头"销售额"默认指它', 'GMV'],
    ['有效支付订单', '有效订单;成交订单', '业务规则', '已支付、非测试单、未在支付后取消', '已下单订单'],
    ['大区', '区域;片区;战区', '维度', '门店的当前归属大区；电商订单无大区', '城市'],
    ['华东', '江浙沪;华东区', '维度值', 'region_code=EC', ''],
    ['华南', '两广;华南区', '维度值', 'region_code=SC；长沙门店 2026-07-01 起不再属于华南', '华中'],
    ['西南', '川渝;西南区', '维度值', 'region_code=SW', ''],
    ['会员', '用户;顾客;客户', '业务对象', '已注册会员；非会员订单 member_id 为空', '顾客'],
    ['复购会员', '回头客;老客', '业务对象', '统计期内有效普通订单不少于2笔的会员', '老会员'],
    ['员工内购', '内购;员工单', '业务规则', 'order_type=staff；计入销售指标，不计入会员指标', ''],
    ['测试单', '测试订单', '业务规则', 'order_type=test；所有指标都必须排除', ''],
    ['自有品牌', '星禾优选;自营品牌', '维度值', 'is_private_label=true', ''],
  ];
  const valueMap = [
    ['orders.status', 'pending_payment', '待支付'], ['orders.status', 'paid', '已支付待发货'], ['orders.status', 'shipped', '已发货'],
    ['orders.status', 'completed', '已完成'], ['orders.status', 'cancelled', '已取消（含支付后取消）'], ['orders.status', 'closed', '超时关闭'],
    ['orders.order_type', 'normal', '普通订单'], ['orders.order_type', 'staff', '员工内购'], ['orders.order_type', 'test', '测试单'],
    ['refunds.status', 'pending', '处理中'], ['refunds.status', 'succeeded', '退款成功'], ['refunds.status', 'rejected', '已驳回'],
    ['members.tier', 'normal', '普通会员'], ['members.tier', 'silver', '银卡'], ['members.tier', 'gold', '金卡'], ['members.tier', 'black', '黑金'],
    ['stores.store_type', 'direct', '直营'], ['stores.store_type', 'franchise', '加盟'],
    ...data.channels.map(c => ['orders.channel_code', c.channel_code, c.channel_name]),
    ...data.regions.map(r => ['stores.region_code', r.region_code, r.region_name]),
  ];
  return {
    'files/store-master.csv': renderCSV(['门店编码', '门店名称', '城市', '当前大区', '经营类型', '面积(平方米)', '开业日期', '闭店日期'], storeRows),
    'files/store-targets-2026.csv': renderCSV(['门店编码', '目标月份', '目标净销售额(元)'], targetRows),
    'files/metric-catalog.csv': renderCSV(['编号', '指标名称', '别名', '业务定义', '计算方式', '口径负责方', '状态'], metricRows),
    'files/business-glossary.xlsx': renderXLSX([
      { name: '术语', header: ['术语', '别名', '类型', '说明', '易混淆'], rows: glossary },
      { name: '字段值映射', header: ['字段', '编码值', '业务含义'], rows: valueMap },
      { name: '指标清单', header: ['编号', '指标名称', '别名', '业务定义', '计算方式', '口径负责方', '状态'], rows: metricRows },
    ]),
  };
}

function dbtFiles() {
  const project = 'retail_lab';
  const sourceTables = ['orders', 'order_items', 'refunds', 'stores', 'members', 'products', 'product_categories'];
  const sources = {}, nodes = {}, catalogSources = {}, catalogNodes = {};
  const columns = {
    orders: [['order_id', 'bigint'], ['member_id', 'bigint'], ['store_id', 'text'], ['channel_code', 'text'], ['order_type', 'text'], ['status', 'text'], ['paid_at', 'timestamp with time zone'], ['goods_amount', 'numeric'], ['paid_amount', 'numeric'], ['shipping_fee', 'numeric']],
    order_items: [['order_id', 'bigint'], ['line_no', 'integer'], ['sku_id', 'text'], ['quantity', 'integer'], ['line_amount', 'numeric'], ['unit_cost', 'numeric']],
    refunds: [['refund_id', 'bigint'], ['order_id', 'bigint'], ['status', 'text'], ['refund_amount', 'numeric'], ['shipping_refund', 'numeric']],
    stores: [['store_id', 'text'], ['region_code', 'text'], ['store_type', 'text']],
    members: [['member_id', 'bigint'], ['tier', 'text'], ['is_employee', 'boolean']],
    products: [['sku_id', 'text'], ['category_id', 'integer'], ['is_private_label', 'boolean']],
    product_categories: [['category_id', 'integer'], ['parent_id', 'integer'], ['category_name', 'text']],
  };
  const catalogColumns = list => Object.fromEntries(list.map(([name, type], i) => [name, { name, index: i + 1, type }]));
  for (const table of sourceTables) {
    const id = `source.${project}.retail.${table}`;
    sources[id] = { database: 'semlia_retail_lab', schema: 'retail', name: table, resource_type: 'source', package_name: project, path: 'models/staging/sources.yml',
      original_file_path: 'models/staging/sources.yml', unique_id: id, fqn: [project, 'staging', 'retail', table], source_name: 'retail', source_description: '星禾零售交易库（虚构）', loader: 'semlia-retail-lab', identifier: table };
    catalogSources[id] = { metadata: { type: 'BASE TABLE', schema: 'retail', name: table }, columns: catalogColumns(columns[table]), stats: {} };
  }
  const models = [
    ['stg_valid_orders', 'staging', "select o.order_id, o.member_id, o.store_id, o.channel_code, o.order_type,\n       o.paid_at, (o.paid_at at time zone 'Asia/Shanghai')::date as paid_date,\n       o.goods_amount, o.paid_amount - o.shipping_fee as goods_paid_amount\nfrom {{ source('retail', 'orders') }} o\nwhere o.paid_at is not null and o.order_type <> 'test' and o.status <> 'cancelled'", ['source.retail_lab.retail.orders'],
      [['order_id', 'bigint'], ['member_id', 'bigint'], ['store_id', 'text'], ['channel_code', 'text'], ['order_type', 'text'], ['paid_at', 'timestamp with time zone'], ['paid_date', 'date'], ['goods_amount', 'numeric'], ['goods_paid_amount', 'numeric']]],
    ['stg_order_refunds', 'staging', "select order_id, sum(refund_amount - shipping_refund) as refund_goods_amount\nfrom {{ source('retail', 'refunds') }}\nwhere status = 'succeeded'\ngroup by order_id", ['source.retail_lab.retail.refunds'],
      [['order_id', 'bigint'], ['refund_goods_amount', 'numeric']]],
    ['fct_order_net_sales', 'marts', "select v.*, coalesce(r.refund_goods_amount, 0) as refund_goods_amount,\n       v.goods_paid_amount - coalesce(r.refund_goods_amount, 0) as net_sales\nfrom {{ ref('stg_valid_orders') }} v\nleft join {{ ref('stg_order_refunds') }} r on r.order_id = v.order_id", ['model.retail_lab.stg_valid_orders', 'model.retail_lab.stg_order_refunds'],
      [['order_id', 'bigint'], ['member_id', 'bigint'], ['store_id', 'text'], ['paid_date', 'date'], ['goods_amount', 'numeric'], ['goods_paid_amount', 'numeric'], ['refund_goods_amount', 'numeric'], ['net_sales', 'numeric']]],
    ['fct_region_daily_sales', 'marts', "select f.paid_date, s.region_code, count(*) as paid_orders,\n       sum(f.goods_amount) as gmv, sum(f.net_sales) as net_sales\nfrom {{ ref('fct_order_net_sales') }} f\njoin {{ source('retail', 'stores') }} s on s.store_id = f.store_id\ngroup by 1, 2", ['model.retail_lab.fct_order_net_sales', 'source.retail_lab.retail.stores'],
      [['paid_date', 'date'], ['region_code', 'text'], ['paid_orders', 'bigint'], ['gmv', 'numeric'], ['net_sales', 'numeric']]],
    ['fct_member_activity', 'marts', "select member_id, min(paid_at) as first_paid_at, max(paid_at) as last_paid_at, count(*) as paid_orders\nfrom {{ ref('stg_valid_orders') }}\nwhere member_id is not null and order_type = 'normal'\ngroup by member_id", ['model.retail_lab.stg_valid_orders'],
      [['member_id', 'bigint'], ['first_paid_at', 'timestamp with time zone'], ['last_paid_at', 'timestamp with time zone'], ['paid_orders', 'bigint']]],
  ];
  for (const [name, folder, code, deps, cols] of models) {
    const id = `model.${project}.${name}`;
    const path = `${folder}/${name}.sql`;
    nodes[id] = { database: 'semlia_retail_lab', schema: 'analytics', name, resource_type: 'model', package_name: project, path, original_file_path: `models/${path}`,
      unique_id: id, fqn: [project, folder, name], alias: name, checksum: { name: 'sha256', checksum: name }, raw_code: code, depends_on: { nodes: deps } };
    catalogNodes[id] = { metadata: { type: 'VIEW', schema: 'analytics', name }, columns: catalogColumns(cols), stats: {} };
  }
  const manifest = { metadata: { dbt_schema_version: 'https://schemas.getdbt.com/dbt/manifest/v12.json', dbt_version: '1.9.0', generated_at: '2026-10-01T00:00:00Z', project_name: project },
    nodes, sources };
  for (const key of ['macros', 'docs', 'exposures', 'metrics', 'groups', 'selectors', 'disabled', 'parent_map', 'child_map', 'group_map', 'saved_queries', 'semantic_models', 'unit_tests']) manifest[key] = {};
  const catalog = { metadata: { dbt_schema_version: 'https://schemas.getdbt.com/dbt/catalog/v1.json', dbt_version: '1.9.0', generated_at: '2026-10-01T00:00:00Z' }, nodes: catalogNodes, sources: catalogSources, errors: null };
  return { 'dbt/manifest.json': JSON.stringify(manifest, null, 2) + '\n', 'dbt/catalog.json': JSON.stringify(catalog, null, 2) + '\n' };
}

// The SQL-bundle source needs the base tables its views read from; reuse the
// CREATE TABLE statements of db/schema.sql so both stay identical.
function tableDefinitions(root) {
  const schema = readFileSync(join(root, 'db/schema.sql'), 'utf8');
  const tables = schema.match(/^CREATE TABLE [\s\S]*?^\);$/gm);
  return '-- Base tables read by sql/mart views. Generated from db/schema.sql; do not edit.\n\n' + tables.join('\n\n') + '\n';
}

export function writeAll(root) {
  const data = generate();
  const expected = expectedAnswers(data);
  const outputs = { 'db/seed.sql': renderSeed(data), 'sql/retail/tables.sql': tableDefinitions(root), ...knowledgeFiles(data), ...dbtFiles(),
    'expected/answers.json': JSON.stringify({ version: data.version, seed: data.seed, cutoff: CUTOFF, answers: expected }, null, 2) + '\n' };
  for (const [path, content] of Object.entries(outputs)) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), content);
  }
  const counts = Object.fromEntries(tables.map(([, t]) => [t, data[t].length]));
  return { files: Object.keys(outputs), counts };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const result = writeAll(dirname(fileURLToPath(import.meta.url)));
  console.log(JSON.stringify(result, null, 2));
}
