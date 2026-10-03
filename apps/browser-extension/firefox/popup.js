const api=typeof browser!=="undefined"?browser:chrome;
const status=document.getElementById("status"),button=document.getElementById("toggle"),detail=document.getElementById("detail");
const accountEl=document.getElementById("account"),pairCreate=document.getElementById("pairCreate"),pairCode=document.getElementById("pairCode");
const pairInput=document.getElementById("pairInput"),pairClaim=document.getElementById("pairClaim");

function send(msg){return new Promise((resolve,reject)=>{api.runtime.sendMessage(msg,r=>{const e=api.runtime.lastError;if(e)reject(e);else resolve(r)})})}

function renderAccount(a){
  if(!a){accountEl.textContent="Пробный или неоплаченный аккаунт";pairCreate.disabled=true;return}
  const name=a.plan_name||a.entitlement||"Доступ";
  accountEl.textContent=name+" · устройства "+a.active_devices+" / "+a.device_limit;
  pairCreate.disabled=Number(a.active_devices)>=Number(a.device_limit);
}

async function refresh(){
  const r=await send({type:"status"});
  if(!r?.ok){status.textContent="Ошибка";button.disabled=true;return}
  status.textContent=r.enabled?"Подключено":"Отключено";
  detail.textContent=r.ingress?("Ingress: "+r.ingress.host+":"+r.ingress.port):"";
  button.textContent=r.enabled?"Отключить":"Подключить";button.disabled=false;button.dataset.enabled=String(r.enabled);
  renderAccount(r.account);
}

button.onclick=async()=>{
  button.disabled=true;
  const enabled=button.dataset.enabled==="true";
  const r=await send({type:enabled?"disconnect":"connect"});
  if(!r?.ok)alert(r?.error||"Ошибка подключения");
  await refresh();
};

pairCreate.onclick=async()=>{
  pairCreate.disabled=true;
  const r=await send({type:"create-pairing-code"});
  if(!r?.ok){alert("Не удалось создать код: "+(r?.error||"ошибка"));await refresh();return}
  pairCode.textContent=r.code+" · до "+r.expires_at;
  await refresh();
};

pairClaim.onclick=async()=>{
  const code=pairInput.value.trim();
  if(!code)return;
  pairClaim.disabled=true;
  const r=await send({type:"claim-pairing-code",code});
  pairClaim.disabled=false;
  if(!r?.ok){alert("Не удалось привязать устройство: "+(r?.error||"ошибка"));return}
  pairInput.value="";pairCode.textContent="";
  await refresh();
};

refresh();
