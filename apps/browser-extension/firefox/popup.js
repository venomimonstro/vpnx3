const api=typeof browser!=="undefined"?browser:chrome;
const status=document.getElementById("status"),button=document.getElementById("toggle"),detail=document.getElementById("detail");
function send(msg){return new Promise((resolve,reject)=>{api.runtime.sendMessage(msg,r=>{const e=api.runtime.lastError;if(e)reject(e);else resolve(r)})})}
async function refresh(){
  const r=await send({type:"status"});
  if(!r?.ok){status.textContent="Ошибка";button.disabled=true;return}
  status.textContent=r.enabled?"Подключено":"Отключено";
  detail.textContent=r.ingress?("Ingress: "+r.ingress.host+":"+r.ingress.port):"";
  button.textContent=r.enabled?"Отключить":"Подключить";button.disabled=false;button.dataset.enabled=String(r.enabled);
}
button.onclick=async()=>{button.disabled=true;const enabled=button.dataset.enabled==="true";const r=await send({type:enabled?"disconnect":"connect"});if(!r?.ok)alert(r?.error||"Ошибка подключения");await refresh()};
refresh();
