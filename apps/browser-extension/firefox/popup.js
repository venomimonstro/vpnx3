const api=typeof browser!=="undefined"?browser:chrome;
const status=document.getElementById("status"),button=document.getElementById("toggle"),detail=document.getElementById("detail");
const account=document.getElementById("account"),createPair=document.getElementById("createPair"),pairCode=document.getElementById("pairCode");
const pairInput=document.getElementById("pairInput"),claimPair=document.getElementById("claimPair");

function send(msg){return new Promise((resolve,reject)=>{api.runtime.sendMessage(msg,r=>{const e=api.runtime.lastError;if(e)reject(e);else resolve(r)})})}

async function refresh(){
  const r=await send({type:"status"});
  if(!r?.ok){status.textContent="Ошибка";button.disabled=true;return}
  status.textContent=r.enabled?"Подключено":"Отключено";
  detail.textContent=r.ingress?("Ingress: "+r.ingress.host+":"+r.ingress.port):"";
  button.textContent=r.enabled?"Отключить":"Подключить";button.disabled=false;button.dataset.enabled=String(r.enabled);
  if(r.account){
    const name=r.account.plan_name||r.account.entitlement;
    account.textContent=name+" · устройства "+r.account.active_devices+"/"+r.account.device_limit+" · до "+r.account.expires_at;
    createPair.disabled=r.account.active_devices>=r.account.device_limit;
  }else{
    account.textContent="Нет активного доступа";
    createPair.disabled=true;
  }
}
button.onclick=async()=>{button.disabled=true;const enabled=button.dataset.enabled==="true";const r=await send({type:enabled?"disconnect":"connect"});if(!r?.ok)alert(r?.error||"Ошибка подключения");await refresh()};
createPair.onclick=async()=>{
  createPair.disabled=true;
  const r=await send({type:"pairing-create"});
  if(r?.ok) pairCode.textContent="Код: "+r.code+" · до "+r.expires;
  else alert("Не удалось создать код");
  await refresh();
};
claimPair.onclick=async()=>{
  const code=pairInput.value.trim();if(!code)return;
  claimPair.disabled=true;
  const r=await send({type:"pairing-claim",code});
  if(!r?.ok) alert("Не удалось привязать устройство");
  else {pairInput.value="";pairCode.textContent="Устройство привязано";}
  claimPair.disabled=false;
  await refresh();
};
refresh();
