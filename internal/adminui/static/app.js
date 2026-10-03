const state={token:sessionStorage.getItem("vpnx3_admin_token")||"",admin:null,section:"dashboard"};

const $=(tag,attrs={},...children)=>{
  const n=document.createElement(tag);
  for(const [k,v] of Object.entries(attrs)){
    if(k==="class") n.className=v;
    else if(k==="onclick") n.addEventListener("click",v);
    else if(k==="onsubmit") n.addEventListener("submit",v);
    else if(k==="value") n.value=v;
    else n.setAttribute(k,v);
  }
  for(const c of children.flat()){
    if(c===null||c===undefined) continue;
    n.append(c instanceof Node?c:document.createTextNode(String(c)));
  }
  return n;
};

async function api(path,options={}){
  const headers=new Headers(options.headers||{});
  headers.set("Accept","application/json");
  if(state.token) headers.set("Authorization","Bearer "+state.token);
  if(options.body && !headers.has("Content-Type")) headers.set("Content-Type","application/json");
  const r=await fetch(path,{...options,headers});
  if(r.status===401){ logout(false); throw new Error("Сессия завершена"); }
  if(!r.ok){
    let msg="HTTP "+r.status;
    try{const j=await r.json();msg=j.detail||j.error||msg}catch{}
    throw new Error(msg);
  }
  if(r.status===204) return null;
  return r.json();
}

async function downloadAuthenticated(path){
  const r=await fetch(path,{headers:{Authorization:"Bearer "+state.token}});
  if(r.status===401){logout(false);throw new Error("Сессия завершена")}
  if(!r.ok){
    let msg="HTTP "+r.status;
    try{const j=await r.json();msg=j.error||msg}catch{}
    throw new Error(msg);
  }
  const disposition=r.headers.get("Content-Disposition")||"";
  const match=disposition.match(/filename="?([^"]+)"?/i);
  const fileName=match?.[1]||"artifact.bin";
  const blob=await r.blob();
  const url=URL.createObjectURL(blob);
  const a=document.createElement("a");
  a.href=url;a.download=fileName;document.body.append(a);a.click();a.remove();
  setTimeout(()=>URL.revokeObjectURL(url),1000);
}

function money(minor,currency="RUB"){
  try{return new Intl.NumberFormat("ru-RU",{style:"currency",currency}).format((minor||0)/100)}
  catch{return ((minor||0)/100).toFixed(2)+" "+currency}
}
function dt(v){return v?new Date(v).toLocaleString("ru-RU"):"—"}
function badge(v){
  const cls=["active","succeeded","resolved"].includes(v)?"good":["degraded","pending","grace","warning"].includes(v)?"warn":["critical","failed","cancelled","quarantined"].includes(v)?"bad":"";
  return $("span",{class:"badge "+cls},v||"—");
}
function can(p){return !!state.admin?.permissions?.includes(p)}

function loginView(message=""){
  const email=$("input",{type:"email",autocomplete:"username",required:"",placeholder:"owner@example.com"});
  const password=$("input",{type:"password",autocomplete:"current-password",required:"",placeholder:"Пароль"});
  const error=$("div",{class:"error"},message);
  const form=$("form",{onsubmit:async e=>{
    e.preventDefault();error.textContent="";
    try{
      const data=await api("/api/v1/admin/login",{method:"POST",body:JSON.stringify({email:email.value,password:password.value})});
      state.token=data.token; state.admin=data.admin;
      sessionStorage.setItem("vpnx3_admin_token",state.token);
      render();
    }catch(err){error.textContent="Вход не выполнен: "+err.message}
  }},
    $("div",{class:"field"},$("label",{},"Email"),email),
    $("div",{class:"field"},$("label",{},"Пароль"),password),
    error,$("button",{class:"btn primary",type:"submit"},"Войти")
  );
  return $("div",{class:"login"},$("section",{class:"card"},$("h1",{},"VPNX3"),$("p",{class:"muted"},"Центр управления сетью"),form));
}

function logout(callApi=true){
  const old=state.token; state.token="";state.admin=null;sessionStorage.removeItem("vpnx3_admin_token");
  if(callApi&&old){fetch("/api/v1/admin/logout",{method:"POST",headers:{Authorization:"Bearer "+old}}).catch(()=>{})}
  document.getElementById("app").replaceChildren(loginView());
}

const sections=[
  ["dashboard","Обзор","analytics.read"],
  ["nodes","Сеть","nodes.read"],
  ["probes","Наблюдение","nodes.read"],
  ["users","Пользователи","users.read"],
  ["billing","Тарифы и платежи","billing.read"],
  ["incidents","Инциденты","incidents.read"],
  ["releases","Релизы","releases.read"],
  ["audit","Аудит","audit.read"]
];

function shell(){
  const nav=$("div",{class:"nav"});
  sections.filter(x=>can(x[2])).forEach(([id,label])=>{
    nav.append($("button",{class:state.section===id?"active":"",onclick:()=>{state.section=id;renderSection()}},label));
  });
  return $("div",{class:"layout"},
    $("aside",{class:"sidebar"},$("div",{class:"brand"},"VPNX3"),nav,
      $("p",{class:"muted"},state.admin.email),
      $("button",{class:"btn",onclick:()=>logout(true)},"Выйти")),
    $("section",{class:"content"},$("div",{id:"section"}))
  );
}

async function ensureAdmin(){
  if(!state.token)return false;
  try{state.admin=await api("/api/v1/admin/me");return true}catch{return false}
}

async function render(){
  const root=document.getElementById("app");
  if(!state.admin && !(await ensureAdmin())){root.replaceChildren(loginView());return}
  root.replaceChildren(shell());
  renderSection();
}

function sectionFrame(title,body){
  return $("div",{},$("div",{class:"topbar"},$("h1",{},title),$("span",{class:"muted"},"Control Plane")),body);
}
function table(headers,rows){
  const thead=$("thead",{},$("tr",{},headers.map(h=>$("th",{},h))));
  const tbody=$("tbody");
  rows.forEach(row=>tbody.append($("tr",{},row.map(c=>$("td",{},c)))));
  return $("div",{class:"table-wrap"},$("table",{},thead,tbody));
}
function sectionError(title,e){return sectionFrame(title,$("div",{class:"card error"},"Ошибка: "+e.message))}

async function dashboard(){
  const d=await api("/api/v1/admin/dashboard");
  const metrics=[
    ["Пользователи",d.users_total],
    ["Активные устройства",d.devices_active],
    ["Активные подписки",d.subscriptions_active],
    ["Активные ноды",d.nodes_active],
    ["Деградированные",d.nodes_degraded],
    ["VPN-сессии",d.current_sessions],
    ["Выручка 30 дней",money(d.revenue_30d_minor)],
    ["Probe success 5 мин",d.probe_success_5m==null?"нет данных":d.probe_success_5m.toFixed(1)+"%"]
  ];
  return sectionFrame("Обзор",$("div",{class:"grid"},metrics.map(([k,v])=>$("div",{class:"card metric"},$("span",{class:"muted"},k),$("strong",{},v)))));
}

async function nodes(){
  const data=await api("/api/v1/nodes");
  const toolbar=$("div",{class:"toolbar"});
  if(can("nodes.manage")){
    toolbar.append($("button",{class:"btn",onclick:enrollmentDialog},"Новый код подключения"));
  }
  if(can("config.manage")){
    toolbar.append($("button",{class:"btn primary",onclick:async()=>{try{await api("/api/v1/config/publish",{method:"POST"});alert("Конфигурация опубликована")}catch(e){alert(e.message)}}},"Опубликовать конфигурацию"));
  }
  const rows=data.nodes.map(n=>[
    $("div",{},$("strong",{},n.name),$("div",{class:"mono muted"},n.id)),
    n.role,badge(n.status),n.country_code||"—",
    n.current_sessions+" / "+(n.capacity_sessions??"—"),
    n.health_score==null?"—":Number(n.health_score).toFixed(1),
    n.circuit_breaker_open?"ОТКРЫТ":"—",dt(n.last_heartbeat_at),
    nodeActions(n)
  ]);
  return sectionFrame("Сеть",$("div",{},toolbar,table(["Нода","Роль","Статус","Страна","Сессии","Health","Circuit","Heartbeat","Действия"],rows)));
}
function nodeActions(n){
  const box=$("div",{class:"row-actions"});
  box.append($("button",{class:"btn",onclick:()=>endpointDialog(n)},"Endpoints"));
  if(can("nodes.manage")){
    const actions=[["active","publish","В работу"],["draining","drain","Drain"],["maintenance","maintenance","Обслуживание"],["quarantined","quarantine","Карантин"],["retired","retire","Списать"]];
    actions.forEach(([,path,label])=>box.append($("button",{class:"btn",onclick:async()=>{try{await api("/api/v1/nodes/"+n.id+"/"+path,{method:"POST",body:JSON.stringify({reason:"admin ui"})});renderSection()}catch(e){alert(e.message)}}},label)));
  }
  return box;
}
async function enrollmentDialog(){
  const role=prompt("Роль: worker / ingress / probe / config_mirror / build_worker","worker");
  if(!role)return;
  try{
    const d=await api("/api/v1/nodes/enrollment-tokens",{method:"POST",body:JSON.stringify({role,ttl_minutes:10})});
    prompt("Одноразовый токен. Скопируйте сейчас:",d.token);
  }catch(e){alert(e.message)}
}
async function endpointDialog(n){
  try{
    const d=await api("/api/v1/nodes/"+n.id+"/endpoints");
    const text=d.endpoints.map(e=>e.id+" | "+e.kind+" | "+e.scheme+"://"+e.host+":"+e.port+e.path).join("\n")||"Endpoints нет";
    if(!can("nodes.manage")){alert(text);return}
    const action=prompt(text+"\n\nВведите add или ID endpoint для удаления","add");
    if(!action)return;
    if(action==="add"){
      const kind=prompt("kind: session_api / wireguard / ingress","session_api"); if(!kind)return;
      const transport=prompt("transport","wireguard"); if(!transport)return;
      const scheme=prompt("scheme: https / udp",kind==="wireguard"?"udp":"https"); if(!scheme)return;
      const host=prompt("host"); if(!host)return;
      const port=Number(prompt("port",kind==="wireguard"?"51820":"443")); if(!port)return;
      const path=kind==="session_api"?(prompt("path","/internal/v1/sessions")||""):"";
      await api("/api/v1/nodes/"+n.id+"/endpoints",{method:"POST",body:JSON.stringify({kind,transport,scheme,host,port,path,priority:100})});
    }else{
      await api("/api/v1/nodes/"+n.id+"/endpoints/"+encodeURIComponent(action),{method:"DELETE"});
    }
    renderSection();
  }catch(e){alert(e.message)}
}

async function probes(){
  const d=await api("/api/v1/probes/recent?limit=200");
  return sectionFrame("Наблюдение",table(["Время","Probe","Target","Тип","Успех","Задержка"],d.results.map(x=>[
    dt(x.observed_at),x.probe_node_id,x.target_node_id,x.endpoint_kind,badge(x.success?"active":"failed"),x.latency_ms+" мс"
  ])));
}

async function users(){
  const d=await api("/api/v1/admin/users?limit=200");
  return sectionFrame("Пользователи",table(["ID","Статус","Устройства","Подписка","До","Создан","Последняя активность","Действие"],d.users.map(u=>[
    $("span",{class:"mono"},u.id),badge(u.status),u.device_count,u.subscription||"trial/нет",dt(u.expires_at),dt(u.created_at),dt(u.last_seen_at),
    $("button",{class:"btn",onclick:()=>userDevices(u)},"Устройства")
  ])));
}
async function userDevices(u){
  try{
    const d=await api("/api/v1/admin/users/"+u.id+"/devices");
    if(!d.devices.length){alert("Устройств нет");return}
    const lines=d.devices.map(x=>x.id+" | "+x.platform+" | "+x.display_name+" | "+x.status+" | last "+dt(x.last_seen_at)).join("\n");
    if(!can("users.manage")){alert(lines);return}
    const cmd=prompt(lines+"\n\nКоманда: revoke <device-id> / reactivate <device-id>","");
    if(!cmd)return;
    const [op,id]=cmd.trim().split(/\s+/,2);
    if(!id)return;
    if(op==="revoke"){
      if(!confirm("Отозвать устройство? Новые Access Lease будут запрещены. Уже активная offline-сессия живёт до своего expires_at."))return;
      await api("/api/v1/admin/users/"+u.id+"/devices/"+encodeURIComponent(id)+"/revoke",{method:"POST"});
    }else if(op==="reactivate"){
      await api("/api/v1/admin/users/"+u.id+"/devices/"+encodeURIComponent(id)+"/reactivate",{method:"POST"});
    }
    renderSection();
  }catch(e){alert(e.message)}
}

async function billing(){
  const [plans,pays]=await Promise.all([api("/api/v1/billing/plans"),api("/api/v1/admin/payments?limit=200")]);
  const create=can("billing.manage")?$("button",{class:"btn primary",onclick:createPlan},"Новая версия тарифа"):null;
  const planTable=table(["Код","Версия","Название","Цена","Период","Устройства","Продажа"],plans.plans.map(p=>[
    p.code,p.version,p.name,money(p.price_minor,p.currency),p.billing_period_days+" дн.",p.device_limit,p.sale_enabled?"да":"нет"
  ]));
  const payTable=table(["Время","Пользователь","Тариф","Провайдер","Статус","Сумма","Оплачен"],pays.payments.map(p=>[
    dt(p.created_at),$("span",{class:"mono"},p.user_id),p.plan_code+" v"+p.plan_version,p.provider,badge(p.status),money(p.amount_minor,p.currency),dt(p.paid_at)
  ]));
  return sectionFrame("Тарифы и платежи",$("div",{class:"stack"},$("div",{class:"toolbar"},create),$("div",{class:"card"},$("h2",{},"Тарифы"),planTable),$("div",{class:"card"},$("h2",{},"Платежи"),payTable)));
}
async function createPlan(){
  const code=prompt("Код тарифа","basic");if(!code)return;
  const name=prompt("Название","Базовый");if(!name)return;
  const price=Number(prompt("Цена, ₽","199"));if(!Number.isFinite(price))return;
  try{
    await api("/api/v1/billing/plans",{method:"POST",body:JSON.stringify({code,name,price_minor:Math.round(price*100),currency:"RUB",billing_period_days:30,device_limit:3,trial_days:7,grace_days:3})});
    renderSection();
  }catch(e){alert(e.message)}
}

async function incidents(){
  const d=await api("/api/v1/admin/incidents?limit=200");
  const toolbar=$("div",{class:"toolbar"});
  if(can("incidents.manage")) toolbar.append($("button",{class:"btn primary",onclick:createIncident},"Создать инцидент"));
  const rows=d.incidents.map(i=>[
    dt(i.detected_at),badge(i.severity),badge(i.status),i.title,i.summary,i.root_cause||"—",
    can("incidents.manage")&&i.status!=="resolved"?$("button",{class:"btn",onclick:async()=>{const root=prompt("Причина/итог","");try{await api("/api/v1/admin/incidents/"+i.id+"/resolve",{method:"POST",body:JSON.stringify({root_cause:root||""})});renderSection()}catch(e){alert(e.message)}}},"Закрыть"):"—"
  ]);
  return sectionFrame("Инциденты",$("div",{},toolbar,table(["Обнаружен","Важность","Статус","Название","Описание","Причина","Действие"],rows)));
}
async function createIncident(){
  const severity=prompt("Важность: info / warning / critical","warning");if(!severity)return;
  const title=prompt("Название");if(!title)return;
  const summary=prompt("Описание");if(!summary)return;
  try{await api("/api/v1/admin/incidents",{method:"POST",body:JSON.stringify({severity,title,summary})});renderSection()}catch(e){alert(e.message)}
}

async function releases(){
  const d=await api("/api/v1/admin/releases?limit=100");
  const toolbar=$("div",{class:"toolbar"});
  if(can("releases.manage")) toolbar.append($("button",{class:"btn primary",onclick:createRelease},"Новый релиз"));
  const rows=d.releases.map(r=>[
    r.version,badge(r.status),$("span",{class:"mono"},r.source_commit),dt(r.created_at),r.notes||"—",
    $("div",{class:"row-actions"},
      $("button",{class:"btn",onclick:async()=>{try{
        const [j,a]=await Promise.all([
          api("/api/v1/admin/releases/"+r.id+"/jobs"),
          api("/api/v1/admin/releases/"+r.id+"/artifacts")
        ]);
        const jobs=j.jobs.map(x=>x.id+" | "+x.target+" — "+x.status+(x.error_summary?" — "+x.error_summary:"")).join("\n")||"Задач нет";
        const arts=a.artifacts.map(x=>x.id+" | "+x.target+" | "+x.file_name+" | "+x.sha256).join("\n")||"Артефактов нет";
        if(!can("releases.manage")){alert(jobs+"\n\n"+arts);return}
        const failed=j.jobs.filter(x=>x.status==="failed"||x.status==="cancelled");
        const cmd=prompt(jobs+"\n\n"+arts+"\n\nКоманда: retry <job-id> / download <artifact-id> / publish / withdraw","");
        if(!cmd)return;
        const [op,id]=cmd.trim().split(/\s+/,2);
        if(op==="retry"&&id) await api("/api/v1/admin/releases/"+r.id+"/jobs/"+encodeURIComponent(id)+"/retry",{method:"POST"});
        else if(op==="publish") await api("/api/v1/admin/releases/"+r.id+"/publish",{method:"POST"});
        else if(op==="withdraw") await api("/api/v1/admin/releases/"+r.id+"/withdraw",{method:"POST"});
        else if(op==="download"&&id){
          await downloadAuthenticated("/api/v1/admin/releases/"+r.id+"/artifacts/"+encodeURIComponent(id)+"/download");
          return;
        } else if(failed.length) alert("Неизвестная команда");
        renderSection();
      }catch(e){alert(e.message)}}},"Управление")
    )
  ]);
  return sectionFrame("Релизы",$("div",{},toolbar,table(["Версия","Статус","Commit","Создан","Заметки","Сборки"],rows)));
}
async function createRelease(){
  const version=prompt("Версия","0.1.0");if(!version)return;
  const source_commit=prompt("Git commit SHA (40 символов)");if(!source_commit)return;
  const raw=prompt("Цели через запятую","android_apk,android_aab");if(!raw)return;
  const targets=raw.split(",").map(x=>x.trim()).filter(Boolean);
  const notes=prompt("Заметки","")||"";
  try{await api("/api/v1/admin/releases",{method:"POST",body:JSON.stringify({version,source_commit,targets,notes})});renderSection()}catch(e){alert(e.message)}
}

async function audit(){
  const d=await api("/api/v1/admin/audit?limit=250");
  return sectionFrame("Аудит",table(["Время","Актор","Действие","Ресурс","Результат","IP","Request ID"],d.events.map(e=>[
    dt(e.created_at),(e.actor_type+" "+(e.actor_id||"")),e.action,e.resource_type+" "+(e.resource_id||""),badge(e.result),e.source_ip||"—",$("span",{class:"mono"},e.request_id||"—")
  ])));
}

async function renderSection(){
  const target=document.getElementById("section");if(!target)return;
  target.replaceChildren($("div",{class:"card"},"Загрузка…"));
  try{
    const fn={dashboard,nodes,probes,users,billing,incidents,releases,audit}[state.section]||dashboard;
    target.replaceChildren(await fn());
  }catch(e){target.replaceChildren(sectionError("VPNX3",e))}
}

render();
