const assert = require('node:assert/strict')
const { scan } = require('./inventory-http-callers.cjs')
const rows = scan(`
const api = '/admin/devices';
sdk.client.fetch<{id:string}>(api, {method:'POST', body:{}});
fetch(\`/store/orders/\${orderId}/timeline\`);
function wrapper(path:string, body:unknown) {return sdk.client.fetch(path,{...(body?{method:'POST'}:{})})}
function shadow(api:string) {return fetch(api)}
{const api='/admin/other';fetch(api,{method:'DELETE'})}
fetch(api,{method: selected});
const options={method:'PATCH'};fetch(api,options);
fetch(api,externalOptions);
query.refetch();
`, 'artificial.ts')
assert.equal(rows.length, 8)
assert.equal(rows[0].endpoint_pattern, '/admin/devices')
assert.deepEqual(rows[0].methods, ['POST'])
assert.equal(rows[1].endpoint_pattern, '/store/orders/{orderId}/timeline')
assert.deepEqual(rows[1].methods, ['GET'])
assert.equal(rows[2].endpoint_pattern, null)
assert.deepEqual(rows[2].methods, [])
assert.equal(rows[3].endpoint_pattern, null, 'parameter shadows file constant')
assert.equal(rows[4].endpoint_pattern, '/admin/other')
assert.deepEqual(rows[4].methods, ['DELETE'])
assert.deepEqual(rows[5].methods, [], 'dynamic method must not become GET')
assert.deepEqual(rows[6].methods, ['PATCH'])
assert.deepEqual(rows[7].methods, [], 'dynamic options must not become GET')
const param = scan(`const opts={method:'POST'};function send(opts:unknown){return fetch('/x',opts)}`, 'parameter.ts')
assert.deepEqual(param[0].methods, [], 'runtime option parameter must not become default GET')
console.log('9 artificial HTTP caller cases passed')
