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
  ["referrals","Рефералы","analytics.read"],
  ["networkrisk","Риск сети","analytics.read"],
  ["fleet","Здоровье нод","analytics.read"],
  ["readiness","Готовность запуска","analytics.read"],
  ["httpmetrics","HTTP метрики","analytics.read"],
  ["issues","Требует внимания","analytics.read"],
  ["nodes","Сеть","nodes.read"],
  ["personalvless","Личный VPN","admin.manage"],
  ["personalopenvpn","OpenVPN","admin.manage"],
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


async function issues(){
  const d=await api("/api/v1/admin/issues?limit=250");
  const summary=$("div",{class:"toolbar"},
    $("span",{class:"badge "+(d.critical?"bad":"good")},"Критичных: "+(d.critical||0)),
    $("span",{class:"badge "+(d.total?"warn":"good")},"Всего: "+(d.total||0))
  );
  if(!d.issues.length){
    return sectionFrame("Требует внимания",$("div",{class:"stack"},
      summary,$("div",{class:"card"},$("strong",{},"Активных проблем не найдено"))
    ));
  }
  const rows=d.issues.map(i=>[
    dt(i.observed_at),badge(i.severity),i.category,i.title,i.detail,
    $("span",{class:"mono"},i.resource_type+" "+i.resource_id)
  ]);
  return sectionFrame("Требует внимания",$("div",{class:"stack"},
    summary,table(["Время","Уровень","Контур","Проблема","Описание","Ресурс"],rows)
  ));
}

async function httpMetrics(){
  const d=await api("/api/v1/admin/http-metrics");
  const m=d.since_process_start||{};
  const rows=[
    ["Активные запросы",m.active||0],
    ["Всего запросов",m.total||0],
    ["2xx",m.status_2xx||0],
    ["3xx",m.status_3xx||0],
    ["4xx",m.status_4xx||0],
    ["5xx",m.status_5xx||0],
    ["Отдано байт",m.bytes_out||0],
  ];
  const latency=m.latency_buckets_ms||{};
  return sectionFrame("HTTP метрики",$("div",{class:"stack"},
    $("div",{class:"card"},$("h2",{},"Текущая реплика"),table(["Показатель","Значение"],rows)),
    $("div",{class:"card"},$("h2",{},"Задержка от старта процесса"),table(["Корзина, мс","Запросы"],
      Object.entries(latency).map(([k,v])=>[k,v])
    )),
    $("p",{class:"muted"},"Метрики агрегированы в памяти текущей Control Plane реплики и не содержат body, query-параметры или историю IP.")
  ));
}

async function readiness(){
  const d=await api("/api/v1/admin/readiness");
  const header=$("div",{class:"card readiness-overall"},
    $("span",{class:"muted"},"Общий статус"),
    badge(d.status),
    $("strong",{},d.status==="ok"?"Можно продолжать запуск":d.status==="warning"?"Есть предупреждения":"Есть блокирующие проблемы"),
    $("span",{class:"muted"},"Проверено: "+dt(d.checked_at))
  );
  const cards=d.checks.map(x=>$("div",{class:"card readiness-check "+x.status},
    $("div",{class:"row-actions"},badge(x.status),$("strong",{},x.title)),
    $("p",{},x.detail),
    $("span",{class:"mono muted"},x.code)
  ));
  const s=d.signals||{};
  const signals=table(["Сигнал","Значение"],[
    ["Active workers",s.active_workers??0],
    ["Routable workers",s.routable_workers??0],
    ["Active ingresses",s.active_ingresses??0],
    ["Active probes",s.active_probes??0],
    ["Fresh probe nodes",s.fresh_probe_nodes??0],
    ["Fresh data-plane workers",s.fresh_data_plane_workers??0],
    ["Queued build jobs",s.queued_build_jobs??0],
    ["Running build jobs",s.running_build_jobs??0],
    ["Active build workers",s.active_build_workers??0],
    ["Published releases",s.published_releases??0],
    ["Runtime-ноды",s.runtime_managed_nodes??0],
    ["Updater telemetry",s.runtime_updater_reported_nodes??0],
    ["Updater с ошибкой",s.runtime_updater_unhealthy_nodes??0],
    ["HA лидер",s.control_plane_leader||"—"],
    ["Heartbeat HA лидера",dt(s.control_plane_leader_heartbeat_at)],
    ["Смен лидерства",s.control_plane_leadership_transitions??0]
  ]);
  return sectionFrame("Готовность запуска",$("div",{class:"stack"},
    header,
    $("div",{class:"readiness-grid"},cards),
    $("div",{class:"card"},$("h2",{},"Сигналы"),signals)
  ));
}


async function referrals(){
  const d=await api("/api/v1/admin/referrals?limit=200");
  const s=d.summary||{};
  const cards=$("div",{class:"kpi-grid"},
    kpi("Активные коды",s.codes_active||0),
    kpi("Claims 30 дней",s.claims_30d||0),
    kpi("Квалифицированы",s.qualified_30d||0),
    kpi("Конверсия",Number(s.qualification_rate||0).toFixed(1)+"%"),
    kpi("Наград выдано",s.granted_rewards_30d||0),
    kpi("Бонусных дней",s.granted_days_30d||0),
    kpi("Возвраты/отзывы",s.reversed_30d||0),
    kpi("Доля отзывов",Number(s.reversal_rate||0).toFixed(1)+"%")
  );
  const rows=(d.redemptions||[]).map(x=>[
    dt(x.claimed_at),x.code,$("span",{class:"mono"},x.referrer_user_id),
    $("span",{class:"mono"},x.referred_user_id),badge(x.status),
    x.reward_days,dt(x.qualified_at),dt(x.reversed_at)
  ]);
  return sectionFrame("Реферальная программа",$("div",{class:"stack"},
    cards,
    $("div",{class:"card"},
      $("h2",{},"Воронка и последние применения"),
      rows.length?table(["Claim","Код","Пригласил","Приглашённый","Статус","Дней","Квалификация","Отзыв"],rows):
        $("div",{class:"empty"},"Применений пока нет")
    )
  ));
}


async function networkrisk(){
  const d=await api("/api/v1/admin/network-risk");
  const cards=$("div",{class:"kpi-grid"},
    kpi("Active worker",d.active_workers||0),
    kpi("Ёмкость",d.configured_capacity||0),
    kpi("Текущие сессии",d.current_sessions||0),
    kpi("Загрузка",Number(d.utilization_percent||0).toFixed(1)+"%"),
    kpi("Запас",Number(d.headroom_percent||0).toFixed(1)+"%"),
    kpi("Без capacity",d.workers_without_capacity||0),
    kpi("Макс. доля провайдера",Number(d.max_provider_share||0).toFixed(1)+"%"),
    kpi("Макс. доля страны",Number(d.max_country_share||0).toFixed(1)+"%")
  );
  const p=(d.providers||[]).map(x=>[
    x.name,x.nodes,x.capacity,x.sessions,Number(x.capacity_share||0).toFixed(1)+"%"
  ]);
  const c=(d.countries||[]).map(x=>[
    x.name,x.nodes,x.capacity,x.sessions,Number(x.capacity_share||0).toFixed(1)+"%"
  ]);
  return sectionFrame("Риск и ёмкость сети",$("div",{class:"stack"},
    cards,
    $("div",{class:"card"},
      $("h2",{},"По провайдерам"),
      p.length?table(["Провайдер","Нод","Ёмкость","Сессии","Доля"],p):$("div",{class:"empty"},"Нет active worker")
    ),
    $("div",{class:"card"},
      $("h2",{},"По странам"),
      c.length?table(["Страна","Нод","Ёмкость","Сессии","Доля"],c):$("div",{class:"empty"},"Нет active worker")
    )
  ));
}


async function fleet(){
  const d=await api("/api/v1/admin/fleet-health");
  const s=d.summary||{};
  const cards=$("div",{class:"kpi-grid"},
    kpi("Нод в наблюдении",s.tracked_nodes||0),
    kpi("Предупреждение",s.local_warning_nodes||0),
    kpi("Авто-degraded",s.local_degraded_nodes||0),
    kpi("Без данных диска",s.missing_disk_telemetry||0),
    kpi("Updater с ошибкой",s.updater_unhealthy_nodes||0)
  );
  const rows=(d.nodes||[]).map(n=>[
    n.name,badge(n.status),n.role,n.provider||"—",n.country_code||"—",
    n.worker_healthy===undefined?"—":badge(n.worker_healthy?"active":"failed"),
    n.disk_total_bytes?Number(n.disk_free_percent||0).toFixed(1)+"%":"—",
    n.uptime_seconds?Math.floor(n.uptime_seconds/3600)+" ч":"—",
    n.local_bad_streak||0,n.local_good_streak||0,
    (n.runtime_updater_unhealthy||0)+"/"+(n.runtime_updater_count||0),
    n.kernel_release||"—",dt(n.last_heartbeat_at)
  ]);
  return sectionFrame("Здоровье нод",$("div",{class:"stack"},
    cards,
    table(["Нода","Статус","Роль","Провайдер","Страна","Worker","Диск свободен","Uptime","Bad","Good","Updater ошибки/всего","Kernel","Heartbeat"],rows)
  ));
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
    ["Активные build-worker",d.build_workers_active||0],
    ["Автопродление включено",d.auto_renew_active||0],
    ["Автоплатежи ожидают",d.renewal_pending||0],
    ["Автопродления успешны 24ч",d.renewal_succeeded_24h||0],
    ["Автопродления с ошибкой 24ч",d.renewal_failed_24h||0],
    ["Автопродление отключено после ошибок",d.renewal_disabled_failures||0],
    ["Рефералы заявлены 30д",d.referrals_claimed_30d||0],
    ["Рефералы квалифицированы 30д",d.referrals_qualified_30d||0],
    ["Бонусных дней выдано 30д",d.referral_reward_days_30d||0]
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
    const renewalRows=(d.renewals||[]).map(r=>[
      dt(r.created_at),dt(r.cycle_expires_at),r.attempt_no,r.provider,
      badge(r.status),r.provider_payment_id||"—",r.error_summary||"—"
    ]);
    const eventRows=(d.events||[]).map(e=>[
      dt(e.created_at),e.action,e.resource_type+" "+(e.resource_id||""),
      badge(e.result),e.request_id||"—"
    ]);
    const signalCards=(d.support_signals||[]).map(s=>$("div",{class:"card readiness-check "+(s.severity==="critical"?"failed":"warning")},
      $("div",{class:"row-actions"},badge(s.severity),$("strong",{},s.title)),
      $("p",{},s.detail),$("span",{class:"mono muted"},s.code)
    ));
    const ref=d.referral||{};

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
        signalCards.length
          ? $("div",{class:"card"},$("h2",{},"Сигналы поддержки"),$("div",{class:"readiness-grid"},signalCards))
          : $("div",{class:"card"},$("h2",{},"Сигналы поддержки"),$("p",{class:"muted"},"Явных проблем не найдено")),
        $("div",{class:"card"},
          $("h2",{},"Реферальная программа"),
          $("p",{},"Код: "+(ref.code||"—")),
          $("p",{},"Приглашён по коду: "+(ref.referred_by||"—")),
          $("p",{},"Заявок 30 дней: "+(ref.claimed_30d||0)),
          $("p",{},"Квалифицировано 30 дней: "+(ref.qualified_30d||0)),
          $("p",{},"Бонусных дней: выдано "+(ref.reward_days_granted||0)+", ожидает "+(ref.reward_days_pending||0)+", отозвано "+(ref.reward_days_revoked||0))),
        $("div",{class:"card"},$("h2",{},"Устройства"),
          table(["Платформа","Название","Статус","Версия","Добавлено","Последняя активность","Действие"],deviceRows)),
        $("div",{class:"card"},$("h2",{},"Последние платежи"),
          paymentRows.length?table(["Время","Тариф","Провайдер","Статус","Сумма","Оплачен"],paymentRows):$("p",{class:"muted"},"Платежей нет")),
        $("div",{class:"card"},$("h2",{},"Автопродление"),
          renewalRows.length?table(["Попытка","Период до","№","Провайдер","Статус","Payment ID","Ошибка"],renewalRows):$("p",{class:"muted"},"Попыток автопродления нет")),
        $("div",{class:"card"},$("h2",{},"Последние события"),
          eventRows.length?table(["Время","Действие","Ресурс","Результат","Request ID"],eventRows):$("p",{class:"muted"},"Событий нет"))
      )
    ));
  }catch(e){
    target.replaceChildren(sectionError("Пользователь",e));
  }
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
  const code=$("input",{value:"basic",placeholder:"basic"});
  const name=$("input",{value:"Базовый",placeholder:"Название"});
  const price=$("input",{type:"number",min:"0",step:"1",value:"199"});
  const period=$("input",{type:"number",min:"1",max:"3650",value:"30"});
  const devices=$("input",{type:"number",min:"1",max:"100",value:"3"});
  const trial=$("input",{type:"number",min:"0",max:"30",value:"7"});
  const grace=$("input",{type:"number",min:"0",max:"30",value:"3"});
  const body=$("div",{},
    field("Код тарифа",code),field("Название",name),field("Цена, ₽",price),
    field("Период, дней",period),field("Лимит устройств",devices),
    field("Пробный период, дней",trial),field("Grace-период, дней",grace)
  );
  modal("Новая версия тарифа",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Создать",primary:true,onclick:async d=>{
      const rub=Number(price.value);
      const payload={
        code:code.value.trim(),name:name.value.trim(),
        price_minor:Math.round(rub*100),currency:"RUB",
        billing_period_days:Number(period.value),device_limit:Number(devices.value),
        trial_days:Number(trial.value),grace_days:Number(grace.value)
      };
      if(!payload.code||!payload.name||!Number.isFinite(rub)||rub<0){alert("Проверьте код, название и цену");return}
      try{
        await api("/api/v1/billing/plans",{method:"POST",body:JSON.stringify(payload)});
        d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function incidents(){
  const [d,notifications]=await Promise.all([
    api("/api/v1/admin/incidents?limit=200"),
    api("/api/v1/admin/incident-notifications")
  ]);
  const ns=notifications.status||{};
  const toolbar=$("div",{class:"toolbar"});
  if(can("incidents.manage")) toolbar.append($("button",{class:"btn primary",onclick:createIncident},"Создать инцидент"));
  const rows=d.incidents.map(i=>[
    dt(i.detected_at),badge(i.severity),badge(i.status),i.title,i.summary,i.root_cause||"—",
    can("incidents.manage")&&i.status!=="resolved"?$("button",{class:"btn",onclick:()=>resolveIncidentDialog(i)},"Закрыть"):"—"
  ]);
  const notificationActions=[];
  if(can("incidents.manage")&&(ns.dead||0)>0){
    notificationActions.push($("button",{class:"btn danger",onclick:async()=>{
      if(!confirm("Повторно поставить недоставленные уведомления в очередь?"))return;
      try{
        const result=await api("/api/v1/admin/incident-notifications/requeue?limit=100",{method:"POST"});
        alert("Возвращено в очередь: "+(result.requeued||0));
        renderSection();
      }catch(e){alert(e.message)}
    }},"Повторить dead-letter"));
  }
  const delivery=$("div",{class:"card"},
    $("div",{class:"card-title-row"},
      $("h2",{},"Доставка уведомлений"),
      $("div",{class:"row-actions"},...notificationActions)
    ),
    $("div",{class:"kpi-grid"},
      kpi("Webhook",notifications.configured?"Настроен":"Не настроен"),
      kpi("В очереди",ns.pending||0),
      kpi("Dead-letter",ns.dead||0),
      kpi("Доставлено",ns.delivered||0)
    )
  );
  return sectionFrame("Инциденты",$("div",{class:"stack"},
    delivery,
    $("div",{},toolbar,table(["Обнаружен","Важность","Статус","Название","Описание","Причина","Действие"],rows))
  ));
}

function resolveIncidentDialog(i){
  const root=$("textarea",{rows:"6",maxlength:"4000",placeholder:"Причина, что исправлено, итог"});
  const body=$("div",{},
    $("p",{},$("strong",{},i.title)),
    field("Причина / итог",root)
  );
  modal("Закрыть инцидент",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Закрыть",primary:true,onclick:async d=>{
      try{
        await api("/api/v1/admin/incidents/"+i.id+"/resolve",{method:"POST",body:JSON.stringify({root_cause:root.value.trim()})});
        d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function releaseDialog(r){
  try{
    const [jobsData,artData,gate]=await Promise.all([
      api("/api/v1/admin/releases/"+r.id+"/jobs"),
      api("/api/v1/admin/releases/"+r.id+"/artifacts"),
      api("/api/v1/admin/releases/"+r.id+"/gate")
    ]);
    const body=$("div",{class:"stack"});
    const gateCard=$("div",{class:"card"},$("h3",{},"Production release gate"),
      $("p",{},"Статус: ",badge(gate.status)),
      $("p",{class:"muted"},gate.enforced
        ?"Production: публикация блокируется критическими проверками."
        :"Не production: проверки информируют, но не блокируют публикацию.")
    );
    (gate.blockers||[]).forEach(x=>gateCard.append($("div",{class:"error"},"Блокер: "+x)));
    (gate.warnings||[]).forEach(x=>gateCard.append($("div",{class:"muted"},"Предупреждение: "+x)));
    body.append(gateCard);
    const jobs=$("div",{class:"card"},$("h3",{},"Задачи сборки"));
    if(!jobsData.jobs.length)jobs.append($("div",{class:"empty"},"Задач нет"));
    jobsData.jobs.forEach(job=>{
      const row=$("div",{class:"endpoint-row"},
        $("div",{},
          $("strong",{},job.target+" · "+job.status),
          $("div",{class:"mono muted"},job.id),
          job.error_summary?$("div",{class:"error"},job.error_summary):null
        )
      );
      if(can("releases.manage")&&(job.status==="failed"||job.status==="cancelled")){
        row.append($("button",{class:"btn",onclick:async()=>{
          try{
            await api("/api/v1/admin/releases/"+r.id+"/jobs/"+encodeURIComponent(job.id)+"/retry",{method:"POST"});
            document.querySelector("dialog.modal")?.close();renderSection();
          }catch(e){alert(e.message)}
        }},"Повторить"));
      }
      jobs.append(row);
    });

    const artifacts=$("div",{class:"card"},$("h3",{},"Артефакты"));
    if(!artData.artifacts.length)artifacts.append($("div",{class:"empty"},"Артефактов нет"));
    artData.artifacts.forEach(a=>{
      artifacts.append($("div",{class:"endpoint-row"},
        $("div",{},
          $("strong",{},a.target+" · "+a.file_name),
          $("div",{class:"mono muted"},a.sha256)
        ),
        $("button",{class:"btn",onclick:()=>downloadAuthenticated(
          "/api/v1/admin/releases/"+r.id+"/artifacts/"+encodeURIComponent(a.id)+"/download"
        ).catch(e=>alert(e.message))},"Скачать")
      ));
    });
    body.append(jobs,artifacts);

    const buttons=[{label:"Закрыть",onclick:d=>d.close()}];
    if(can("releases.manage")&&r.status==="ready"&&!(gate.enforced&&(gate.blockers||[]).length)){
      buttons.push({label:"Опубликовать",primary:true,onclick:async d=>{
        try{await api("/api/v1/admin/releases/"+r.id+"/publish",{method:"POST"});d.close();renderSection()}catch(e){alert(e.message)}
      }});
    }
    if(can("releases.manage")&&r.status==="published"){
      buttons.push({label:"Отозвать",danger:true,onclick:async d=>{
        if(!confirm("Отозвать опубликованный релиз?"))return;
        try{await api("/api/v1/admin/releases/"+r.id+"/withdraw",{method:"POST"});d.close();renderSection()}catch(e){alert(e.message)}
      }});
    }
    modal("Релиз "+r.version,body,buttons);
  }catch(e){alert(e.message)}
}

function adminEditDialog(a){
  const roles=$("input",{value:(a.roles||[]).join(","),placeholder:"roles через запятую"});
  const status=$("select",{},
    $("option",{value:"active"},"active"),
    $("option",{value:"disabled"},"disabled")
  );
  status.value=a.status;
  const body=$("div",{},
    $("p",{},$("strong",{},a.email)),
    field("Роли",roles),
    field("Статус",status),
    $("p",{class:"muted"},"Сервер не позволит снять роль/отключить последнего активного owner.")
  );
  modal("Администратор",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Сохранить",primary:true,onclick:async d=>{
      try{
        const roleList=roles.value.split(",").map(x=>x.trim()).filter(Boolean);
        if(!roleList.length){alert("Нужна минимум одна роль");return}
        const rolesChanged=JSON.stringify([...roleList].sort())!==JSON.stringify([...(a.roles||[])].sort());
        if(rolesChanged){
          await api("/api/v1/admin/admins/"+a.id+"/roles",{method:"PUT",body:JSON.stringify({roles:roleList})});
        }
        if(status.value!==a.status){
          await api("/api/v1/admin/admins/"+a.id+"/status",{method:"PUT",body:JSON.stringify({status:status.value})});
        }
        d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function createIncident(){
  const severity=$("select",{},
    ...["info","warning","critical"].map(v=>$("option",{value:v},v))
  );severity.value="warning";
  const title=$("input",{maxlength:"200",placeholder:"Краткое название"});
  const summary=$("textarea",{rows:"6",maxlength:"4000",placeholder:"Что произошло, влияние, текущий статус"});
  const body=$("div",{},field("Важность",severity),field("Название",title),field("Описание",summary));
  modal("Создать инцидент",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Создать",primary:true,onclick:async d=>{
      if(!title.value.trim()||!summary.value.trim()){alert("Заполните название и описание");return}
      try{
        await api("/api/v1/admin/incidents",{method:"POST",body:JSON.stringify({
          severity:severity.value,title:title.value.trim(),summary:summary.value.trim()
        })});
        d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function releases(){
  const d=await api("/api/v1/admin/releases?limit=100");
  const toolbar=$("div",{class:"toolbar"});
  if(can("releases.manage")) toolbar.append(
    $("button",{class:"btn primary",onclick:createRelease},"Новый релиз"),
    $("button",{class:"btn",onclick:releasePolicyDialog},"Политика версий")
  );
  const rows=d.releases.map(r=>[
    r.version,badge(r.status),$("span",{class:"mono"},r.source_commit),dt(r.created_at),r.notes||"—",
    $("div",{class:"row-actions"},
      $("button",{class:"btn",onclick:()=>releaseDialog(r)},"Управление")
    )
  ]);
  return sectionFrame("Релизы",$("div",{},toolbar,table(["Версия","Статус","Commit","Создан","Заметки","Сборки"],rows)));
}

async function releasePolicyDialog(){
  const targets=[
    ["android_apk","Android APK"],
    ["ios_ipa","iOS IPA"],
    ["chrome_zip","Chrome"],
    ["firefox_zip","Firefox"]
  ];
  const target=$("select",{},...targets.map(([value,label])=>$("option",{value},label)));
  const minimum=$("input",{placeholder:"например 1.4.0"});
  const recommended=$("input",{placeholder:"например 1.5.0"});
  const rollout=$("input",{type:"number",min:"0",max:"100",value:"100"});
  const blocked=$("input",{placeholder:"1.2.0, 1.3.1"});
  const message=$("textarea",{rows:"4",maxlength:"500",placeholder:"Сообщение пользователю"});
  const status=$("div",{class:"muted"},"Загрузка политики…");

  async function load(){
    try{
      const p=await api("/api/v1/admin/release-policy?target="+encodeURIComponent(target.value));
      minimum.value=p.minimum_supported_version||"";
      recommended.value=p.recommended_version||"";
      rollout.value=String(p.rollout_percent??100);
      blocked.value=(p.blocked_versions||[]).join(", ");
      message.value=p.message||"";
      status.textContent="Обновлено: "+dt(p.updated_at);
    }catch(e){status.textContent=e.message}
  }
  target.addEventListener("change",load);

  const body=$("div",{},
    field("Клиент",target),
    field("Минимально поддерживаемая версия",minimum),
    field("Рекомендуемая версия",recommended),
    field("Раскатка, % устройств",rollout),
    field("Заблокированные версии через запятую",blocked),
    field("Сообщение пользователю",message),
    $("p",{class:"muted"},
      "Заблокированные и версии ниже минимума всегда требуют обновление. Процент раскатки влияет только на рекомендуемую версию."
    ),
    status
  );

  const dialog=modal("Политика версий",body,[
    {label:"Закрыть",onclick:d=>d.close()},
    {label:"Сохранить",primary:true,onclick:async()=>{
      const percent=Number(rollout.value);
      if(!Number.isInteger(percent)||percent<0||percent>100){
        alert("Процент раскатки должен быть от 0 до 100");return
      }
      try{
        const p=await api("/api/v1/admin/release-policy",{
          method:"PUT",
          body:JSON.stringify({
            target:target.value,
            minimum_supported_version:minimum.value.trim(),
            recommended_version:recommended.value.trim(),
            rollout_percent:percent,
            blocked_versions:blocked.value.split(",").map(x=>x.trim()).filter(Boolean),
            message:message.value.trim()
          })
        });
        status.textContent="Сохранено: "+dt(p.updated_at);
      }catch(e){alert(e.message)}
    }}
  ]);
  await load();
}
async function createRelease(){
  const version=$("input",{value:"0.1.0",placeholder:"0.1.0"});
  const commit=$("input",{placeholder:"40-символьный Git commit SHA",maxlength:"40"});
  const targets=$("input",{value:"android_apk,android_aab",placeholder:"android_apk,android_aab"});
  const notes=$("textarea",{rows:"4",placeholder:"Что входит в релиз"});
  const body=$("div",{},
    field("Версия",version),field("Git commit",commit),
    field("Цели сборки через запятую",targets),field("Заметки",notes)
  );
  modal("Новый релиз",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Создать",primary:true,onclick:async d=>{
      const source_commit=commit.value.trim().toLowerCase();
      const list=targets.value.split(",").map(x=>x.trim()).filter(Boolean);
      if(!/^([0-9a-f]{40})$/.test(source_commit)){alert("Git commit должен содержать 40 hex-символов");return}
      if(!version.value.trim()||!list.length){alert("Укажите версию и цели сборки");return}
      try{
        await api("/api/v1/admin/releases",{method:"POST",body:JSON.stringify({
          version:version.value.trim(),source_commit,targets:list,notes:notes.value.trim()
        })});
        d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function audit(){
  const [d,external]=await Promise.all([
    api("/api/v1/admin/audit?limit=250"),
    api("/api/v1/admin/security-export")
  ]);
  const s=external.status||{};
  const actions=[];
  if(can("security.export.manage")&&(s.dead||0)>0){
    actions.push($("button",{class:"btn danger",onclick:async()=>{
      if(!confirm("Повторно поставить dead-letter события в очередь?"))return;
      try{
        const result=await api("/api/v1/admin/security-export/requeue?limit=100",{method:"POST"});
        alert("Возвращено в очередь: "+(result.requeued||0));
        renderSection();
      }catch(e){alert(e.message)}
    }},"Повторить dead-letter"));
  }
  const externalCard=$("div",{class:"card"},
    $("div",{class:"card-title-row"},
      $("h2",{},"Внешний журнал безопасности"),
      $("div",{class:"row-actions"},...actions)
    ),
    $("div",{class:"kpi-grid"},
      kpi("Настроен",external.configured?"Да":"Нет"),
      kpi("В очереди",s.pending||0),
      kpi("Dead-letter",s.dead||0),
      kpi("Доставлено",s.delivered||0)
    ),
    $("p",{class:"muted"},
      s.last_delivered_at
        ?"Последняя доставка: "+dt(s.last_delivered_at)
        :"Успешных доставок пока нет"
    )
  );
  return sectionFrame("Аудит",$("div",{class:"stack"},
    externalCard,
    table(["Время","Актор","Действие","Ресурс","Результат","IP","Request ID"],d.events.map(e=>[
      dt(e.created_at),(e.actor_type+" "+(e.actor_id||"")),e.action,
      e.resource_type+" "+(e.resource_id||""),badge(e.result),
      e.source_ip||"—",$("span",{class:"mono"},e.request_id||"—")
    ]))
  ));
}

async function admins(){
  const d=await api("/api/v1/admin/admins");
  const toolbar=$("div",{class:"toolbar"},$("button",{class:"btn primary",onclick:createAdmin},"Новый администратор"));
  const rows=d.admins.map(a=>[
    a.email,badge(a.status),a.roles.join(", "),dt(a.created_at),
    a.id===state.admin.id?"текущая учётная запись":
      $("button",{class:"btn",onclick:()=>adminEditDialog(a)},"Изменить")
  ]);
  return sectionFrame("Администраторы",$("div",{},toolbar,table(["Email","Статус","Роли","Создан","Действие"],rows)));
}
async function createAdmin(){
  const email=$("input",{type:"email",autocomplete:"off",placeholder:"admin@example.com"});
  const password=$("input",{type:"password",autocomplete:"new-password",placeholder:"Минимум 14 символов"});
  const roles=$("input",{value:"read_only",placeholder:"read_only или несколько ролей через запятую"});
  const body=$("div",{},
    field("Email",email),field("Временный пароль",password),field("Роли",roles),
    $("p",{class:"muted"},"Роли проверяются сервером. Неизвестная роль будет отклонена.")
  );
  modal("Новый администратор",body,[
    {label:"Отмена",onclick:d=>d.close()},
    {label:"Создать",primary:true,onclick:async d=>{
      const roleList=roles.value.split(",").map(x=>x.trim()).filter(Boolean);
      if(!email.value.trim()||password.value.length<14||!roleList.length){
        alert("Проверьте email, пароль (минимум 14 символов) и роли");return;
      }
      try{
        await api("/api/v1/admin/admins",{method:"POST",body:JSON.stringify({
          email:email.value.trim(),password:password.value,roles:roleList
        })});
        password.value="";d.close();renderSection();
      }catch(e){alert(e.message)}
    }}
  ]);
}

async function personalVless(){
  const d=await api("/api/v1/admin/personal-vless");
  const healthy=!!(d.running&&d.local_port_listening);
  const primaryPort=d.https_443_available?443:(d.port||8443);
  const checkResult=$("p",{class:"muted"},"Локальный TCP-порт не гарантирует доступ с мобильного оператора. Проверьте ключ и затем устройство.");
  const testButton=$("button",{class:"btn primary",onclick:async()=>{
    testButton.disabled=true;
    checkResult.textContent="Проверяется реальный VLESS / REALITY / HTTPS...";
    try{
      const result=await api("/api/v1/admin/personal-vless/check",{method:"POST"});
      checkResult.className="success";
      checkResult.textContent=result.verified
        ?"Подключение VLESS + REALITY и HTTPS работают (локальный тест сервера)."
        :"Не удалось подтвердить VPN-подключение.";
    }catch(err){
      checkResult.className="error";
      checkResult.textContent="VPN-проверка не прошла: "+err.message;
    }finally{testButton.disabled=false}
  }},"Проверить VPN-подключение");
  const status=$( "div",{class:"card"},
    $("h2",{},"Личный VPN · VLESS + REALITY"),
    $("div",{class:"toolbar"},
      badge(healthy?"active":"failed"),
      $("span",{class:"mono"},(d.address||"194.146.223.104")+":"+primaryPort),
      d.https_443_available?$("span",{class:"muted"},"443 · основной, 8443 · резервный"):$("span",{class:"muted"},"Ожидается внешний тест доступности из РФ"),
      $("button",{class:"btn",onclick:()=>renderSection()},"Обновить состояние"),
      $("button",{class:"btn",onclick:async()=>{
        if(!confirm("Добавить резервный публичный VLESS-порт 2053/TCP? Xray кратковременно перезапустится, ключи сохранятся."))return;
        try{await api("/api/v1/admin/personal-vless/alternate-port",{method:"POST"});renderSection()}
        catch(e){alert("Не удалось открыть резервный порт: "+e.message)}
      }},"Включить порт 2053"),
      testButton
    ),
    $("p",{class:"muted"},healthy
      ?"Xray работает, TCP-порт отвечает локально. Внешнее соединение необходимо проверить в Happ/v2ray."
      :"Xray остановлен или порт недоступен локально. Проверьте сервис personal-vless-manager и Docker."),
    checkResult,
    $("div",{class:"card"},
      $("h3",{},"Telegram на iPhone · проверка маршрутизации INCY"),
      $("p",{class:"muted"},"Если VPN включён, но Telegram не загружается, сначала исключите правила split tunneling. Это меняет только маршруты в приложении INCY, серверные ключи не трогаются."),
      $("div",{class:"toolbar"},
        $("button",{class:"btn primary",onclick:()=>{
          if(confirm("Отключить пользовательские правила маршрутизации INCY на этом iPhone? Существующие профили останутся сохранены.")){
            window.location.href="incy://routing/off";
          }
        }},"INCY: отключить split-маршрутизацию"),
        $("button",{class:"btn",onclick:()=>window.open("https://docs.incy.cc/routing/","_blank","noopener,noreferrer")},"Документация INCY")
      ),
      $("p",{class:"muted"},"После переключения полностью отключите и включите VPN в INCY, закройте и снова откройте Telegram. Для проверки сравните Wi-Fi и мобильный интернет. Это не гарантирует доступность Telegram при блокировках в сети.")
    ),
    $("p",{class:"muted"},"INCY поддерживает VLESS + REALITY. На iPhone проверьте в настройках iOS разрешение VPN, выключите второй VPN/Private Relay для диагностики, проверьте профиль на Wi-Fi и мобильной сети. Наличие ключа в INCY ещё не подтверждает работу сети."),
    $("p",{class:"muted"},"Если локальный тест ключа успешен, а iPhone не подключается, необходимо проверить внешнюю доступность TCP-порта у хостинга и оператора."),
    $("p",{class:"muted"},"Ключи видны только администраторам. Отзыв ключа отключает устройство и перезапускает Xray на несколько секунд.")
  );
  const actions=$("div",{class:"toolbar"});
  const newName=$("input",{placeholder:"Например: iPhone, Android",maxlength:"64",style:"min-width:200px"});
  const mode=$("select",{},
    $("option",{value:"vision"},"Android / ПК · XTLS Vision"),
    $("option",{value:"ios"},"iPhone / iPad · Совместимый REALITY")
  );
  actions.append(newName,mode,$("button",{class:"btn primary",onclick:async()=>{
    const name=newName.value.trim();
    if(!name){alert("Укажите имя устройства");return}
    try{
      await api("/api/v1/admin/personal-vless",{method:"POST",body:JSON.stringify({name,mode:mode.value})});
      renderSection()
    }catch(err){alert("Не удалось создать ключ: "+err.message)}
  }},"Создать VLESS-ключ"));
  const profiles=d.profiles||[];
  const rows=profiles.map(p=>[
    $("div",{},$("strong",{},p.name||"Устройство"),$("div",{class:"muted mono"},p.id)),
    $("div",{},$("div",{},p.mode==="ios"?"iOS · REALITY":"Vision · Android/ПК"),
      $("span",{class:"muted"},p.validation?.valid
        ?(healthy?"Ссылка проверена · порт отвечает локально":"Ссылка проверена · сервер недоступен")
        :"ОШИБКА ССЫЛКИ: "+(p.validation?.issues||["unknown"]).join(", "))),
    $("div",{class:"row-actions"},
      $("button",{class:"btn",onclick:async()=>{
        try{
          const result=await api("/api/v1/admin/personal-vless/"+encodeURIComponent(p.id)+"/check",{method:"POST"});
          alert("Профиль "+p.name+": VLESS + REALITY + HTTPS работают на сервере. Время локального теста: "+result.local_test_ms+" мс. Это не измерение скорости мобильной сети.");
        }catch(e){alert("Проверка ключа не прошла: "+e.message)}
      }},"Тест ключа"),
      $("button",{class:"btn",disabled:!p.uri,onclick:()=>navigator.clipboard.writeText(p.uri).then(()=>alert("Ссылка скопирована")).catch(()=>showPersonalVless(p))},"Скопировать"),
      $("button",{class:"btn",disabled:!p.uri,onclick:()=>showPersonalVless(p)},"Показать ссылку"),
      ...(p.fallback_uri?[ $("button",{class:"btn",onclick:()=>navigator.clipboard.writeText(p.fallback_uri).then(()=>alert("Резервный VLESS 8443 скопирован"))},"Резервный 8443") ]:[]),
      ...(p.https_uri?[ $("button",{class:"btn primary",onclick:()=>{
        const content=$("div",{class:"stack"},
          $("p",{class:"muted"},"TLS-порт 443 — общий с админкой. Импортируйте эту ссылку в совместимый VLESS-клиент. Ключ не публикуйте."),
          $("textarea",{rows:"5",readonly:"",class:"mono",style:"width:100%"})
        );
        content.querySelector("textarea").value=p.https_uri;
        modal("VPN через 443/TCP · "+p.name,content,[
          {label:"Закрыть",onclick:d=>d.close()},
          {label:"Копировать",primary:true,onclick:()=>navigator.clipboard.writeText(p.https_uri)}
        ]);
      }},"VPN 443 · основной") ]:[]),
      ...(p.alternate_uri?[ $("button",{class:"btn primary",onclick:()=>{
        const content=$("div",{class:"stack"},
          $("p",{class:"muted"},"Резервный адрес 2053/TCP. Важно: порт должен быть открыт у хостинга. После импорта проверьте подключение с iPhone."),
          $("textarea",{rows:"5",readonly:"",class:"mono",style:"width:100%"})
        );
        content.querySelector("textarea").value=p.alternate_uri;
        modal("Резервный ключ iPhone · 2053",content,[
          {label:"Закрыть",onclick:d=>d.close()},
          {label:"Копировать",primary:true,onclick:()=>navigator.clipboard.writeText(p.alternate_uri)}
        ]);
      }},"Резервный ключ 2053") ]:[]),
      $("button",{class:"btn primary",disabled:!p.uri,onclick:()=>{
        // INCY's documented native direct-import route. No third-party API receives this key.
        if(typeof p.uri!=="string"||!p.uri.startsWith("vless://")){alert("Некорректный ключ");return}
        window.location.href="incy://add/"+p.uri;
      }},"Открыть в INCY"),
      $("button",{class:"btn danger",onclick:async()=>{
        if(!confirm("Отозвать ключ устройства «"+p.name+"»? Подключение перестанет работать."))return;
        try{await api("/api/v1/admin/personal-vless/"+encodeURIComponent(p.id),{method:"DELETE"});renderSection()}
        catch(err){alert("Ошибка отзыва: "+err.message)}
      }},"Отозвать")
    )
  ]);
  return sectionFrame("Личный VPN", $("div",{class:"stack"},status,
    $("div",{class:"card"},$("h2",{},"Мои устройства"),actions,
      $("p",{class:"muted"},"Для iOS создавайте отдельный профиль REALITY без XTLS Vision. После активации общего шлюза новый импорт автоматически использует порт 443; старый 8443 остаётся резервным. Проверяйте реальное подключение через мобильную сеть и Wi-Fi."),
      profiles.length?table(["Устройство","Статус сервера","Ключ и управление"],rows)
       :$("div",{class:"empty"},"Ключи ещё не созданы")
    )
  ));
}
function showPersonalVless(p){
  const content=$("div",{class:"stack"},
    $("p",{class:"muted"},"Импортируйте эту ссылку в Happ, v2rayNG или другой клиент с поддержкой VLESS + REALITY."),
    $("textarea",{readonly:"",rows:"5",class:"mono",style:"width:100%;word-break:break-all"})
  );
  content.querySelector("textarea").value=p.uri||"";
  modal("Ключ: "+p.name,content,[
    {label:"Закрыть",onclick:d=>d.close()},
    {label:"Копировать",primary:true,onclick:()=>navigator.clipboard.writeText(p.uri)}
  ]);
}

async function personalOpenVPN(){
  const data=await api("/api/v1/admin/personal-openvpn");
  const status=$("div",{class:"card"},
    $("h2",{},"OpenVPN · профили устройств"),
    $("div",{class:"toolbar"},badge(data.ready?"active":"failed"),$("span",{},data.ready?"Сервер и сертификаты готовы":data.running?"Сервер запущен, но есть проблемы готовности":"Сервер не запущен")),
    $("p",{class:"muted"},"UDP "+(data.port||1194)+": "+(data.listener_open?"слушается":"не прослушивается")+" · Сертификат: "+(data.certificate_valid?"OK":"проблема")+" · Список отзыва (CRL): "+(data.crl_valid?"OK":"проблема")+". Это не подтверждает подключение из России."),
    $("p",{class:"muted"},data.installed
      ?"OpenVPN установлен. Профили содержат приватные ключи — храните файлы только на своих устройствах."
      :"OpenVPN ещё не установлен на сервере. Выполните sudo bash scripts/install-personal-openvpn.sh install. Порт 1194/UDP должен быть открыт у хостинга.")
  );
  const input=$("input",{placeholder:"Например: iphone",maxlength:"40",autocomplete:"off"});
  const create=$("button",{class:"btn primary",onclick:async()=>{
    const name=input.value.trim();
    if(!/^[A-Za-z][A-Za-z0-9_-]{0,39}$/.test(name)){alert("Имя: латинские буквы, цифры, _ и -");return}
    create.disabled=true;
    try{await api("/api/v1/admin/personal-openvpn",{method:"POST",body:JSON.stringify({name})});renderSection()}
    catch(e){alert("Создание OpenVPN-профиля: "+e.message)}
    finally{create.disabled=false}
  }},"Создать профиль");
  const rows=(data.profiles||[]).map(p=>[
    $("strong",{},p.name),
    $("div",{class:"row-actions"},
      $("button",{class:"btn primary",onclick:async()=>{
        try{await downloadAuthenticated("/api/v1/admin/personal-openvpn/"+encodeURIComponent(p.name))}
        catch(e){alert("Ошибка скачивания: "+e.message)}
      }},"Скачать .ovpn"),
      $("button",{class:"btn danger",onclick:async()=>{
        if(!confirm("Отозвать сертификат "+p.name+"? Устройство потеряет доступ."))return;
        try{await api("/api/v1/admin/personal-openvpn/"+encodeURIComponent(p.name),{method:"DELETE"});renderSection()}
        catch(e){alert("Ошибка отзыва: "+e.message)}
      }},"Отозвать")
    )
  ]);
  return sectionFrame("OpenVPN", $("div",{class:"stack"},status,$("div",{class:"card"},
    $("h2",{},"Сертификаты и устройства"),
    $("div",{class:"toolbar"},input,create),
    rows.length?table(["Устройство","Действия"],rows):$("p",{class:"muted"},"Нет созданных профилей"),
    $("p",{class:"muted"},"Файл .ovpn импортируется в OpenVPN Connect на iOS или Android. Работа в конкретной мобильной сети требует отдельной проверки.")
  )));
}

async function renderSection(){
  const target=document.getElementById("section");if(!target)return;
  target.replaceChildren($("div",{class:"card"},"Загрузка…"));
  try{
    const fn={dashboard,referrals,networkrisk,fleet,readiness,httpmetrics:httpMetrics,issues,nodes,personalvless:personalVless,personalopenvpn:personalOpenVPN,probes,telemetry,users,billing,incidents,releases,audit,admins}[state.section]||dashboard;
    target.replaceChildren(await fn());
  }catch(e){target.replaceChildren(sectionError("VPNX3",e))}
}

render();
