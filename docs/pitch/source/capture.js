const puppeteer = require('puppeteer-core');
(async () => {
  const b = await puppeteer.launch({ executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', headless: true });
  const p = await b.newPage();
  await p.setViewport({ width: 1600, height: 900, deviceScaleFactor: 2 });
  await p.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
  await p.goto('https://llm-reliability-console.vercel.app/stale/check', { waitUntil: 'networkidle2', timeout: 90000 });
  const click = async (t) => p.evaluate((t) => { const el=[...document.querySelectorAll('button,a,[role=button]')].find(b=>b.textContent.trim().includes(t)); el && el.click(); return !!el; }, t);
  await p.waitForFunction(()=>document.body.innerText.includes('Load example'),{timeout:60000}); console.log('load', await click('Load example')); await new Promise(r=>setTimeout(r,800)); console.log('len', await p.evaluate(()=>document.querySelector('textarea').value.length));
  await new Promise(r=>setTimeout(r,500));
  console.log('check', await click('Check answer'));
  for (let i=0;i<40;i++){ await new Promise(r=>setTimeout(r,700)); if (p.url().includes('/reports/')) break; await p.screenshot({ path: `shots/run-${String(i).padStart(2,'0')}.png` }); }
  console.log('url', p.url());
  await p.waitForFunction(() => document.body.innerText.includes('Corrected answer'), { timeout: 60000 });
  await new Promise(r=>setTimeout(r,1500));
  await p.screenshot({ path: 'shots/report.png' });
  await b.close();
})().catch(e=>{console.error(e);process.exit(1)});
