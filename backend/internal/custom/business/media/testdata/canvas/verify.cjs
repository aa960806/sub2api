// Execute the actual public Canvas Seedance functions against the Go test
// server, with a local upstream fixture. No production API key or paid calls.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const baseUrl = process.argv[2];
assert.match(baseUrl, /^http:\/\/127\.0\.0\.1:\d+$/);
const requests = [];
const axios = {
  async post(url, body, options) { return this.request('POST', url, body, options); },
  async get(url, options) { return this.request('GET', url, undefined, options); },
  async request(method, url, body, options = {}) {
    assert.ok(url.startsWith(baseUrl + '/'), 'network must stay on local fixture');
    requests.push({method, url, body, options});
    const response = await fetch(url, {method, headers:options.headers, body:body ? JSON.stringify(body) : undefined});
    const data = options.responseType === 'blob' ? await response.blob() : await response.json();
    if (!response.ok) { const error = Error('HTTP ' + response.status); error.response={status:response.status,data}; error.isAxiosError=true; throw error; }
    return {data};
  },
  isAxiosError(error) { return !!error.isAxiosError; },
};
const context = vm.createContext({URL, Blob, Error, FormData, setTimeout, fetch:() => {throw Error('Unexpected direct fetch');}});
const readFunction = name => vm.runInContext('(' + fs.readFileSync(path.join(__dirname, name), 'utf8') + ')', context, {timeout:1000});
const configHelpers = {
  buildApiUrl:readFunction('build-api-url.js'),
  modelOptionName:name => name,
  resolveModelRequestConfig:(config, model) => ({...config, model}),
};
const seedance = {}, video = {};
const exportTo = out => values => { for (let i=0;i<values.length;i+=3) out[values[i]]=values[i+2]; };
readFunction('seedance-helpers.js')({i:id => {
  if (id===56420) return {default:()=>null}; // unrelated icon from the shared chunk
  assert.equal(id,56073); return configHelpers;
},s:exportTo(seedance)});
const deps = {
  81949:{default:axios},
  28928:{dataUrlToFile:() => {throw Error('Seedance should not use multipart');}},
  23241:{uploadMediaFile:() => {throw Error('Storage is outside this contract');}},
  7065:{imageToDataUrl:async image => image.dataUrl},
  94299:seedance,
  56073:configHelpers,
};
readFunction('video-module.js')({i:id => {assert.ok(deps[id]); return deps[id];},s:exportTo(video)});

(async () => {
  const config = {baseUrl,apiKey:'canvas-fixture-key',model:'seedance2.0',videoModel:'seedance2.0',videoSeconds:'5',size:'16:9',vquality:'720',videoGenerateAudio:'true',videoWatermark:'false'};
  const models = (await axios.get(configHelpers.buildApiUrl(baseUrl,'/models'),{headers:{Authorization:'Bearer '+config.apiKey}})).data.data.map(m=>m.id);
  assert.ok(models.includes(config.model));
  const png='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aOWQAAAAASUVORK5CYII=';
  for (const images of [[],[{dataUrl:'data:image/png;base64,'+png},{dataUrl:'data:image/png;base64,'+png}]]) {
    const task=await video.createVideoGenerationTask(config,'a cat riding a bicycle',images);
    assert.equal(task.provider,'seedance');
    assert.ok(task.id);
    const request=requests.at(-1);
    assert.equal(request.url,baseUrl+'/v1/contents/generations/tasks');
    assert.equal(request.options.headers['Idempotency-Key'],undefined);
    assert.equal(request.body.content.length,1+images.length);
    let poll;
    for(let i=0;i<4;i++) {
      poll=await video.pollVideoGenerationTask(config,task);
      if(poll.status==='completed') break;
      assert.equal(poll.status,'pending');
    }
    assert.equal(poll.status,'completed');
    assert.ok(poll.result.blob, 'must download video, not fall back to a broken URL');
    assert.equal(await poll.result.blob.text(),'MP4-FIXTURE');
    const download=requests.at(-1);
    assert.match(download.url,/\/v1\/media\/results\/.+signature=/);
    assert.equal(download.options.headers,undefined,'actual Canvas download carries no Authorization');
  }
  await assert.rejects(video.createVideoGenerationTask({...config,videoSeconds:'6'},'test'),/supports durations/);
  await assert.rejects(video.createVideoGenerationTask({...config,vquality:'1080p'},'test'),/only supports 720p/);
  await assert.rejects(video.createVideoGenerationTask(config,'test',[],[{url:'https://example.invalid/reference.mp4'}]),/reference images only/);
  console.log('CANVAS_SEEDANCE_CONTRACT=passed (models, text, two inline images, polling, unauthenticated signed download, unsupported inputs)');
})().catch(error => { console.error(error);process.exitCode=1; });
