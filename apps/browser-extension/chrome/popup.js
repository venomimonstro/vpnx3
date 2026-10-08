const api=typeof browser!=="undefined"?browser:chrome;
const status=document.getElementById("status"),updateEl=document.getElementById("update");
const button=document.getElementById("toggle"),detail=document.getElementById("detail");
const devicesEl=document.getElementById("devices");
const accountEl=document.getElementById("account"),pairCreate=document.getElementById("pairCreate"),pairCode=document.getElementById("pairCode");
const pairInput=document.getElementById("pairInput"),pairClaim=document.getElementById("pairClaim");
const referralCode=document.getElementById("referralCode"),referralState=document.getElementById("referralState");
const referralInput=document.getElementById("referralInput"),referralClaim=document.getElementById("referralClaim");

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
  const u=r.update||{};
  updateEl.textContent=u.required
    ?(u.message||"Требуется обновить расширение")
    :(u.availableVersion?("Доступно обновление "+u.availableVersion):"");
  button.textContent=r.enabled?"Отключить":(u.required?"Требуется обновление":"Подключить");
  button.disabled=!!u.required&&!r.enabled;
  button.dataset.enabled=String(r.enabled);
  renderAccount(r.account);
  devicesEl.replaceChildren();
  for(const d of (r.devices||[])){
    const row=document.createElement("div");
    row.className="device-row";
    const text=document.createElement("span");
    text.textContent=d.display_name+(d.current?" · это устройство":"")+" · "+d.platform;
    row.appendChild(text);
    if(!d.current&&d.status==="active"){
      const revoke=document.createElement("button");
      revoke.className="secondary";
      revoke.textContent="Отключить";
      revoke.onclick=async()=>{
        revoke.disabled=true;
        const result=await send({type:"device-revoke",deviceId:d.id});
        if(!result?.ok)alert("Не удалось отключить устройство: "+(result?.error||"ошибка"));
        await refresh();
      };
      row.appendChild(revoke);
    }
    devicesEl.appendChild(row);
  }
  const ref=await send({type:"referral-status"}).catch(()=>null);
  if(ref?.ok){
    referralCode.textContent=ref.code?("Ваш код: "+ref.code):"";
    const s=ref.status||{};
    const pending=Number(s.pending_rewards||0),granted=Number(s.granted_rewards||0),reversed=Number(s.reversed_rewards||0);
    referralState.textContent="Ожидают: "+pending+" · начислено: "+granted+(reversed?(" · отозвано: "+reversed):"");
  }
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
  const r=await send({type:"pairing-create"});
  if(!r?.ok){alert("Не удалось создать код: "+(r?.error||"ошибка"));await refresh();return}
  pairCode.textContent=r.code+" · до "+r.expires;
  await refresh();
};

pairClaim.onclick=async()=>{
  const code=pairInput.value.trim();if(!code)return;
  pairClaim.disabled=true;
  const r=await send({type:"pairing-claim",code});
  pairClaim.disabled=false;
  if(!r?.ok){alert("Не удалось привязать устройство: "+(r?.error||"ошибка"));return}
  pairInput.value="";pairCode.textContent="";
  await refresh();
};

referralClaim.onclick=async()=>{
  const code=referralInput.value.trim();if(!code)return;
  referralClaim.disabled=true;
  const r=await send({type:"referral-claim",code});
  referralClaim.disabled=false;
  if(!r?.ok){alert("Не удалось применить код: "+(r?.error||"ошибка"));return}
  referralInput.value="";
  await refresh();
};

refresh().catch(()=>{status.textContent="Ошибка";button.disabled=true});
