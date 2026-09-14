const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');

async function harness() {
  const elements = new Map();
  const element = id => {
    if (!elements.has(id)) elements.set(id, {
      value:'', innerHTML:'', textContent:'', hidden:false, open:false, listeners:{}, elements:{},
      contains:()=>false, addEventListener(type,fn){this.listeners[type]=fn},
      showModal(){this.open=true}, close(){this.open=false}, setAttribute(){},
    });
    return elements.get(id);
  };
  for(const name of ['id','title','project','minutes','status','resume','waitingOn','checkDate','mit','weekly']) {
    element('taskForm').elements[name]={value:'',checked:false};
  }
  let state={data:{version:1,tasks:[{id:'a',title:'Original',project:'Demo',status:'ready',minutes:30,source:'manual',weekly:false}],settings:{zone:'Europe/Moscow',reserve:25},review:{}},schedule:{date:'2026-09-14',blocks:[],warnings:[],free:0,budget:0},candidates:[]};
  const writes=[];
  const context={
    document:{getElementById:element,querySelectorAll:()=>[],activeElement:null},
    window:{addEventListener(){}}, localStorage:{getItem:()=>null}, setInterval(){},
    Intl, Date, escapeHTML:v=>String(v??'').replaceAll('<','&lt;'), empty:v=>v,
    formatDateTime:new Intl.DateTimeFormat('ru-RU'),
    fetch:async(url,options)=>{
      if(options.method==='POST') {
        const command=JSON.parse(options.body);writes.push(command);
        if(command.version!==state.data.version) return {ok:false,status:409,text:async()=> 'conflict'};
        state.data.tasks=[command.task];state.data.version++;
      }
      return {ok:true,json:async()=>structuredClone(state)};
    },
  };
  vm.runInNewContext(readFileSync('web/workspace.js','utf8'),context);
  await new Promise(resolve=>setImmediate(resolve));
  const click = dataset=>element('workspace').listeners.click({target:{closest:()=>({dataset})}});
  return {element,click,writes,state,load:async()=>{await element('workDate').listeners.change()}};
}

test('conflicted editor cannot overwrite a newer task on retry',async()=>{
  const h=await harness();
  await h.click({edit:'a'});
  h.element('taskForm').elements.title.value='My edit';
  h.state.data.version=2;h.state.data.tasks[0].title='Another tab';
  const submit=()=>h.element('taskForm').listeners.submit({preventDefault(){},target:h.element('taskForm')});
  await submit();await submit();
  assert.deepEqual(h.writes.map(w=>w.version),[1,1]);
  assert.equal(h.state.data.tasks[0].title,'Another tab');
  assert.equal(h.element('taskDialog').open,true);
  await h.click({edit:'a'});
  h.element('taskForm').elements.title.value='Merged edit';
  await submit();
  assert.equal(h.writes[2].version,2);
  assert.equal(h.state.data.tasks[0].title,'Merged edit');
  assert.equal(h.element('taskDialog').open,false);
});

test('completed weekly task does not make a project active',async()=>{
  const h=await harness();h.state.data.tasks[0].status='done';h.state.data.tasks[0].weekly=true;
  await h.load();
  assert.doesNotMatch(h.element('projectWarnings').innerHTML,/Demo:/);
  h.state.data.tasks[0].status='waiting';await h.load();
  assert.match(h.element('projectWarnings').innerHTML,/Demo:/);
});

test('missing calendar capacity is distinct from confirmed zero free time',async()=>{
  const h=await harness();
  assert.match(h.element('workMetrics').innerHTML,/Нет данных/);
  h.state.schedule.available=true;await h.load();
  assert.doesNotMatch(h.element('workMetrics').innerHTML,/Нет данных/);
  assert.match(h.element('workMetrics').innerHTML,/0 мин/);
});
