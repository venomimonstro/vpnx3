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
  ["telemetry","Клиентская диагностика","analytics.read"],
  ["users","Пользователи","users.read"],
  ["billing","Тарифы и платежи","billing.read"],
  ["incidents","Инциденты","incidents.read"],
  ["releases","Релизы","releases.read"],
  ["audit","Аудит","audit.read"],
  ["admins","Администраторы","admin.manage"]
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


function modal(title,content,buttons=[]){
  const dialog=$("dialog",{class:"modal"});
  const actions=$("div",{class:"modal-actions"});
  buttons.forEach(b=>actions.append($("button",{class:"btn "+(b.primary?"primary":b.danger?"danger":""),type:"button",onclick:()=>b.onclick(dialog)},b.label)));
  dialog.append($("div",{class:"modal-card"},$("h2",{},title),content,actions));
  document.body.append(dialog);
  dialog.addEventListener("close",()=>dialog.remove(),{once:true});
  dialog.addEventListener("click",e=>{if(e.target===dialog)dialog.close()});
  dialog.showModal();
  return dialog;
}

function field(label,input){
  return $("div",{class:"field"},$("label",{},label),input);
}

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
    ["Probe success 5 мин",d.probe_success_5m==null?"нет данных":d.probe_success_5m.toFixed(1)+"%"],
    ["Блокировки регистраций 24ч",d.registration_blocks_24h||0],
    ["Блокировки платежей 24ч",d.payment_blocks_24h||0],
    ["Сборки в очереди",d.build_queued||0],
    ["Сборки выполняются",d.build_running||0],
    ["Сборки с ошибкой",d.build_failed||0],
    ["Активные build-worker",d.build_workers_active||0]
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
    const actions=[
      ["draft","approve","Принять тест"],
      ["active","publish","В работу"],
      ["draining","drain","Drain"],
      ["maintenance","maintenance","Обслуживание"],
      ["quarantined","quarantine","Карантин"],
      ["retired","retire","Списать"]
    ];
    actions.forEach(([,path,label])=>box.append($("button",{class:"btn",onclick:async()=>{try{await api("/api/v1/nodes/"+n.id+"/"+path,{method:"POST",body:JSON.stringify({reason:"admin ui"})});renderSection()}catch(e){alert(e.message)}}},label)));
  }
  return box;
}
async function enrollmentDialog(){
  const role=$("select",{},
    ...["worker","ingress","probe","config_mirror","build_worker"].map(v=>$("option",{value:v},v))
  );
  const ttl=$("input",{type:"number",min:"1",max:"60",value:"10"});
  const result=$("textarea",{readonly:"",rows:"4",placeholder:"После создания здесь появится одноразовый токен"});
  const body=$("div",{},
    field("Роль ноды",role),
    field("Срок действия, минут",ttl),
    field("Одноразовый токен",result),
    $("p",{class:"muted"},"Токен показывается для копирования и используется только один раз.")
  );
  modal("Новый код подключения",body,[
    {label:"Закрыть",onclick:d=>d.close()},
    {label:"Создать",primary:true,onclick:async d=>{
      try{
        const minutes=Number(ttl.value);
        if(!Number.isInteger(minutes)||minutes<1||minutes>60){alert("Срок должен быть 1–60 минут");return}
        const data=await api("/api/v1/nodes/enrollment-tokens",{method:"POST",body:JSON.stringify({role:role.value,ttl_minutes:minutes})});
        result.value=data.token;
        result.focus();result.select();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function endpointDialog(n){
  try{
    const data=await api("/api/v1/nodes/"+n.id+"/endpoints");
    const list=$("div",{class:"stack"});
    const renderList=()=>{
      list.replaceChildren();
      if(!data.endpoints.length){
        list.append($("div",{class:"empty"},"Endpoints нет"));
        return;
      }
      data.endpoints.forEach(ep=>{
        const row=$("div",{class:"endpoint-row"},
          $("div",{},
            $("strong",{},ep.kind+" · "+ep.scheme+" · "+ep.transport),
            $("div",{class:"mono muted"},ep.host+":"+ep.port+(ep.path||"")+" · priority "+ep.priority)
          )
        );
        if(can("nodes.manage")){
          row.append($("button",{class:"btn danger",onclick:async()=>{
            if(!confirm("Удалить endpoint "+ep.host+":"+ep.port+"?"))return;
            try{
              await api("/api/v1/nodes/"+n.id+"/endpoints/"+encodeURIComponent(ep.id),{method:"DELETE"});
              data.endpoints=data.endpoints.filter(x=>x.id!==ep.id);
              renderList();
            }catch(e){alert(e.message)}
          }},"Удалить"));
        }
        list.append(row);
      });
    };
    renderList();

    if(!can("nodes.manage")){
      modal("Endpoints · "+n.name,list,[{label:"Закрыть",onclick:d=>d.close()}]);
      return;
    }

    const kind=$("select",{},
      ...["session_api","wireguard","ingress"].map(v=>$("option",{value:v},v))
    );
    const transport=$("input",{value:"wireguard",placeholder:"transport"});
    const scheme=$("select",{},
      $("option",{value:"https"},"https"),
      $("option",{value:"udp"},"udp")
    );
    const host=$("input",{placeholder:"vpn.example.net или IP"});
    const port=$("input",{type:"number",min:"1",max:"65535",value:"443"});
    const path=$("input",{value:"/internal/v1/sessions",placeholder:"/internal/v1/sessions"});
    const priority=$("input",{type:"number",min:"1",max:"10000",value:"100"});

    const syncDefaults=()=>{
      if(kind.value==="wireguard"){
        scheme.value="udp";transport.value="wireguard";port.value="51820";path.value="";
      }else if(kind.value==="ingress"){
        scheme.value="https";transport.value="https_connect";port.value="443";path.value="";
      }else{
        scheme.value="https";transport.value="wireguard";port.value="443";path.value="/internal/v1/sessions";
      }
    };
    kind.addEventListener("change",syncDefaults);

    const form=$("div",{class:"stack"},
      list,
      $("div",{class:"card"},
        $("h3",{},"Добавить endpoint"),
        field("Тип",kind),
        field("Транспорт",transport),
        field("Схема",scheme),
        field("Хост",host),
        field("Порт",port),
        field("Путь",path),
        field("Приоритет",priority)
      )
    );
    modal("Endpoints · "+n.name,form,[
      {label:"Закрыть",onclick:d=>d.close()},
      {label:"Добавить",primary:true,onclick:async()=>{
        try{
          const payload={
            kind:kind.value,
            transport:transport.value.trim(),
            scheme:scheme.value,
            host:host.value.trim(),
            port:Number(port.value),
            path:path.value.trim(),
            priority:Number(priority.value)
          };
          if(!payload.host||!Number.isInteger(payload.port)||payload.port<1||payload.port>65535){
            alert("Проверьте host и port");return;
          }
          const ep=await api("/api/v1/nodes/"+n.id+"/endpoints",{method:"POST",body:JSON.stringify(payload)});
          data.endpoints.push(ep);
          renderList();
          host.value="";
        }catch(e){alert(e.message)}
      }}
    ]);
  }catch(e){alert(e.message)}
}


async function probes(){
  const d=await api("/api/v1/probes/recent?limit=200");
  return sectionFrame("Наблюдение",table(["Время","Probe","Target","Тип","Успех","Задержка"],d.results.map(x=>[
    dt(x.observed_at),x.probe_node_id,x.target_node_id,x.endpoint_kind,badge(x.success?"active":"failed"),x.latency_ms+" мс"
  ])));
}

async function telemetry(){
  const d=await api("/api/v1/admin/telemetry?days=14");
  return sectionFrame("Клиентская диагностика",table(
    ["День","Платформа","Версия","Событие","Config","Worker","Сеть","Количество","Среднее время"],
    d.metrics.map(x=>[
      dt(x.day),x.platform,x.client_version,x.event_type,x.config_version,
      x.worker_node_id||"—",x.network_type,x.event_count,
      Math.round(x.average_duration_ms)+" мс"
    ])
  ));
}

async function users(){
  const q=$("input",{type:"search",placeholder:"ID, email, телефон, устройство"});
  const status=$("select",{},
    $("option",{value:""},"Все статусы"),
    $("option",{value:"active"},"active"),
    $("option",{value:"disabled"},"disabled"),
    $("option",{value:"blocked"},"blocked")
  );
  const body=$("div");
  const load=async()=>{
    body.replaceChildren($("div",{class:"card"},"Загрузка…"));
    const params=new URLSearchParams({limit:"200"});
    if(q.value.trim())params.set("q",q.value.trim());
    if(status.value)params.set("status",status.value);
    const d=await api("/api/v1/admin/users?"+params.toString());
    body.replaceChildren(table(["ID","Статус","Устройства","Подписка","До","Создан","Последняя активность","Действие"],d.users.map(u=>[
      $("span",{class:"mono"},u.id),badge(u.status),u.device_count,u.subscription||"trial/нет",dt(u.expires_at),dt(u.created_at),dt(u.last_seen_at),
      $("button",{class:"btn",onclick:()=>userDetail(u.id)},"Карточка")
    ])));
  };
  q.addEventListener("keydown",e=>{if(e.key==="Enter")load().catch(err=>alert(err.message))});
  status.addEventListener("change",()=>load().catch(err=>alert(err.message)));
  const search=$("button",{class:"btn primary",onclick:()=>load().catch(err=>alert(err.message))},"Найти");
  await load();
  return sectionFrame("Пользователи",$("div",{class:"stack"},
    $("div",{class:"toolbar"},q,status,search),body
  ));
}

async function userDetail(userId){
  const target=document.getElementById("section");if(!target)return;
  target.replaceChildren($("div",{class:"card"},"Загрузка карточки…"));
  try{
    const d=await api("/api/v1/admin/users/"+userId);
    const u=d.user;
    const back=$("button",{class:"btn",onclick:()=>renderSection()},"← К пользователям");

    const sub=u.subscription
      ? $("div",{class:"card"},
          $("h2",{},"Подписка"),
          $("p",{},"Тариф: "+u.subscription.plan_name+" ("+u.subscription.plan_code+" v"+u.subscription.plan_version+")"),
          $("p",{}, "Статус: ", badge(u.subscription.status)),
          $("p",{},"Действует до: "+dt(u.subscription.expires_at)),
          $("p",{},"Grace до: "+dt(u.subscription.grace_until)),
          $("p",{},"Устройства: "+u.active_device_count+" / "+u.subscription.device_limit),
          $("p",{},"Автопродление: "+(u.subscription.auto_renew?"да":"нет")))
      : $("div",{class:"card"},
          $("h2",{},"Доступ"),
          $("p",{},"Активной подписки нет"),
          $("p",{},"Trial до: "+dt(u.trial_expires_at)),
          $("p",{},"Активные устройства: "+u.active_device_count));

    const deviceRows=d.devices.map(dev=>[
      dev.platform,
      dev.display_name,
      badge(dev.status),
      dev.client_version||"—",
      dt(dev.first_seen_at),
      dt(dev.last_seen_at),
      can("users.manage")
        ? $("div",{class:"row-actions"},
            dev.status==="active"
              ? $("button",{class:"btn",onclick:async()=>{
                  if(!confirm("Отозвать это устройство?"))return;
                  try{
                    await api("/api/v1/admin/users/"+u.id+"/devices/"+encodeURIComponent(dev.id)+"/revoke",{method:"POST"});
                    await userDetail(u.id);
                  }catch(e){alert(e.message)}
                }},"Отозвать")
              : $("button",{class:"btn",onclick:async()=>{
                  try{
                    await api("/api/v1/admin/users/"+u.id+"/devices/"+encodeURIComponent(dev.id)+"/reactivate",{method:"POST"});
                    await userDetail(u.id);
                  }catch(e){
                    if(e.message==="device_limit_reached") alert("Нельзя активировать: достигнут лимит устройств тарифа");
                    else alert(e.message);
                  }
                }},"Активировать"))
        : "—"
    ]);

    const paymentRows=d.payments.map(p=>[
      dt(p.created_at),p.plan_code+" v"+p.plan_version,p.provider,
      badge(p.status),money(p.amount_minor,p.currency),dt(p.paid_at)
    ]);

    target.replaceChildren(sectionFrame("Пользователь",
      $("div",{class:"stack"},
        $("div",{class:"toolbar"},back),
        $("div",{class:"card"},
          $("h2",{},"Профиль"),
          $("p",{},$("span",{class:"mono"},u.id)),
          $("p",{},"Статус: ",badge(u.status)),
          $("p",{},"Email: "+(u.email||"—")),
          $("p",{},"Телефон: "+(u.phone||"—")),
          $("p",{},"Создан: "+dt(u.created_at)),
          $("p",{},"Последняя активность: "+dt(u.last_seen_at))),
        sub,
        $("div",{class:"card"},$("h2",{},"Устройства"),
          table(["Платформа","Название","Статус","Версия","Добавлено","Последняя активность","Действие"],deviceRows)),
        $("div",{class:"card"},$("h2",{},"Последние платежи"),
          paymentRows.length?table(["Время","Тариф","Провайдер","Статус","Сумма","Оплачен"],paymentRows):$("p",{class:"muted"},"Платежей нет"))
      )
    ));
  }catch(e){
    target.replaceChildren(sectionError("Пользователь",e));
  }
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


function financeChart(rows){
  if(!rows?.length)return $("div",{class:"empty"},"Нет финансовых данных");
  const max=Math.max(1,...rows.map(x=>Math.max(x.captured_minor||0,x.refunded_minor||0,Math.abs(x.net_minor||0))));
  const bars=rows.map(x=>{
    const captured=Math.max(2,Math.round(((x.captured_minor||0)/max)*100));
    const refunded=Math.max(0,Math.round(((x.refunded_minor||0)/max)*100));
    const label=new Date(x.day).toLocaleDateString("ru-RU",{day:"2-digit",month:"2-digit"});
    return $("div",{class:"finance-day",title:
      label+" · поступления "+money(x.captured_minor)+" · возвраты "+money(x.refunded_minor)+" · чистыми "+money(x.net_minor)},
      $("div",{class:"finance-bars"},
        $("div",{class:"finance-bar captured",style:"height:"+captured+"%"}),
        refunded?$("div",{class:"finance-bar refunded",style:"height:"+Math.max(2,refunded)+"%"}):null
      ),
      $("span",{},label)
    );
  });
  return $("div",{class:"finance-chart",role:"img","aria-label":"Финансовая динамика по дням"},bars);
}

async function billing(){
  const paymentQ=$("input",{type:"search",placeholder:"User ID, payment ID, тариф"});
  const paymentStatus=$("select",{},
    $("option",{value:""},"Все статусы"),
    ...["pending","succeeded","failed","refunded","cancelled"].map(x=>$("option",{value:x},x))
  );
  const paymentProvider=$("input",{type:"search",placeholder:"Провайдер"});
  const paymentBody=$("div");

  const [plans,finance]=await Promise.all([
    api("/api/v1/billing/plans"),
    api("/api/v1/admin/finance/summary?days=30")
  ]);
  const loadPayments=async()=>{
    const params=new URLSearchParams({limit:"200"});
    if(paymentQ.value.trim())params.set("q",paymentQ.value.trim());
    if(paymentStatus.value)params.set("status",paymentStatus.value);
    if(paymentProvider.value.trim())params.set("provider",paymentProvider.value.trim());
    const pays=await api("/api/v1/admin/payments?"+params.toString());
    paymentBody.replaceChildren(table(["Время","Пользователь","Тариф","Провайдер","Статус","Сумма","Оплачен"],pays.payments.map(p=>[
      dt(p.created_at),$("span",{class:"mono"},p.user_id),p.plan_code+" v"+p.plan_version,p.provider,badge(p.status),money(p.amount_minor,p.currency),dt(p.paid_at)
    ])));
  };
  paymentQ.addEventListener("keydown",e=>{if(e.key==="Enter")loadPayments().catch(err=>alert(err.message))});
  paymentStatus.addEventListener("change",()=>loadPayments().catch(err=>alert(err.message)));
  const create=can("billing.manage")?$("button",{class:"btn primary",onclick:createPlan},"Новая версия тарифа"):null;
  const planTable=table(["Код","Версия","Название","Цена","Период","Устройства","Продажа"],plans.plans.map(p=>[
    p.code,p.version,p.name,money(p.price_minor,p.currency),p.billing_period_days+" дн.",p.device_limit,p.sale_enabled?"да":"нет"
  ]));
  await loadPayments();
  const financeCard=$("div",{class:"card"},$("h2",{},"Финансы за 30 дней"),
    $("div",{class:"finance-summary"},
      $("div",{},$("span",{class:"muted"},"Поступления"),$("strong",{},money(finance.captured_minor,finance.currency))),
      $("div",{},$("span",{class:"muted"},"Возвраты"),$("strong",{},money(finance.refunded_minor,finance.currency))),
      $("div",{},$("span",{class:"muted"},"Чистыми"),$("strong",{},money(finance.net_minor,finance.currency))),
      $("div",{},$("span",{class:"muted"},"Успешные оплаты"),$("strong",{},finance.succeeded_payments||0))
    ),
    financeChart(finance.daily));
  return sectionFrame("Тарифы и платежи",$("div",{class:"stack"},
    $("div",{class:"toolbar"},create),
    financeCard,
    $("div",{class:"card"},$("h2",{},"Тарифы"),planTable),
    $("div",{class:"card"},
      $("h2",{},"Платежи"),
      $("div",{class:"toolbar"},paymentQ,paymentStatus,paymentProvider,
        $("button",{class:"btn",onclick:()=>loadPayments().catch(err=>alert(err.message))},"Найти")),
      paymentBody)
  ));
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

async function admins(){
  const d=await api("/api/v1/admin/admins");
  const toolbar=$("div",{class:"toolbar"},$("button",{class:"btn primary",onclick:createAdmin},"Новый администратор"));
  const rows=d.admins.map(a=>[
    a.email,badge(a.status),a.roles.join(", "),dt(a.created_at),
    a.id===state.admin.id?"текущая учётная запись":
      $("button",{class:"btn",onclick:async()=>{
        const cmd=prompt("Команда: roles role1,role2 / status active|disabled","");
        if(!cmd)return;
        try{
          if(cmd.startsWith("roles ")){
            const roles=cmd.slice(6).split(",").map(x=>x.trim()).filter(Boolean);
            await api("/api/v1/admin/admins/"+a.id+"/roles",{method:"PUT",body:JSON.stringify({roles})});
          }else if(cmd.startsWith("status ")){
            await api("/api/v1/admin/admins/"+a.id+"/status",{method:"PUT",body:JSON.stringify({status:cmd.slice(7).trim()})});
          }
          renderSection();
        }catch(e){alert(e.message)}
      }},"Изменить")
  ]);
  return sectionFrame("Администраторы",$("div",{},toolbar,table(["Email","Статус","Роли","Создан","Действие"],rows)));
}
async function createAdmin(){
  const email=prompt("Email");if(!email)return;
  const password=prompt("Временный пароль (минимум 14 символов)");if(!password)return;
  const raw=prompt("Роли через запятую","read_only");if(!raw)return;
  const roles=raw.split(",").map(x=>x.trim()).filter(Boolean);
  try{await api("/api/v1/admin/admins",{method:"POST",body:JSON.stringify({email,password,roles})});renderSection()}catch(e){alert(e.message)}
}

async function renderSection(){
  const target=document.getElementById("section");if(!target)return;
  target.replaceChildren($("div",{class:"card"},"Загрузка…"));
  try{
    const fn={dashboard,nodes,probes,telemetry,users,billing,incidents,releases,audit,admins}[state.section]||dashboard;
    target.replaceChildren(await fn());
  }catch(e){target.replaceChildren(sectionError("VPNX3",e))}
}

render();
