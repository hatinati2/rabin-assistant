const api = globalThis.browser ?? globalThis.chrome;
const status = document.getElementById("status");
document.getElementById("fill").addEventListener("click", async () => {
  let order;
  try {
    order = JSON.parse(document.getElementById("order").value);
    if (!order || typeof order !== "object" || !order.symbol || !["buy","sell"].includes(order.side) ||
        !Number.isFinite(Number(order.price)) || !Number.isFinite(Number(order.quantity))) {
      throw new Error("نماد، side (buy/sell)، قیمت و تعداد را کامل وارد کن.");
    }
  } catch (e) { status.textContent = "اطلاعات نامعتبر است: " + e.message; return; }
  try {
    const [tab] = await api.tabs.query({active:true,currentWindow:true});
    if (!tab?.id || !/^https?:/.test(tab.url || "")) throw new Error("ابتدا صفحه سفارش رابین را باز کن.");
    const results = await api.scripting.executeScript({target:{tabId:tab.id},func:fillOrderFields,args:[order]});
    status.textContent = results?.[0]?.result || "اجرا شد؛ فرم را بررسی کن.";
  } catch(e) { status.textContent = "خطا: " + e.message; }
});
function fillOrderFields(order) {
  const norm = s => (s || "").replace(/[يى]/g,"ی").replace(/ك/g,"ک").replace(/\s+/g," ").trim().toLowerCase();
  const fields = [...document.querySelectorAll("input:not([type=hidden]):not([type=submit]), select")];
  const hints = {symbol:["نماد","symbol","کد نماد"],price:["قیمت","price"],quantity:["تعداد","quantity","حجم"],side:["نوع معامله","نوع سفارش","side"]};
  const labelOf = el => {
    const label = el.id ? document.querySelector('label[for="'+CSS.escape(el.id)+'"]') : null;
    return norm([label?.innerText,el.getAttribute("aria-label"),el.placeholder,el.name,el.id,el.closest("label")?.innerText].filter(Boolean).join(" "));
  };
  const setValue = (el,value) => {
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el),"value")?.set;
    if (setter) setter.call(el,String(value)); else el.value=String(value);
    el.dispatchEvent(new Event("input",{bubbles:true}));el.dispatchEvent(new Event("change",{bubbles:true}));
  };
  const filled=[];
  for (const key of ["symbol","price","quantity"]) {
    const el=fields.find(x=>hints[key].some(h=>labelOf(x).includes(norm(h))));
    if(el){setValue(el,order[key]);filled.push(key);}
  }
  const sideField=fields.find(x=>hints.side.some(h=>labelOf(x).includes(norm(h))));
  if(sideField && sideField.tagName==="SELECT"){
    const option=[...sideField.options].find(o=>norm(o.value+" "+o.text).includes(order.side==="buy"?"خرید":"فروش") || norm(o.value+" "+o.text).includes(order.side));
    if(option){sideField.value=option.value;sideField.dispatchEvent(new Event("change",{bubbles:true}));filled.push("side");}
  }
  return filled.length ? "فیلدهای شناسایی‌شده پر شدند: "+filled.join("، ")+".\nفیلدهای باقی‌مانده را دستی تکمیل کن و همه اطلاعات را بررسی کن."
    : "فیلدی با برچسب قابل تشخیص پیدا نشد. فرم رابین هنوز برای افزونه تنظیم نشده؛ هیچ اطلاعاتی تغییر نکرد.";
}