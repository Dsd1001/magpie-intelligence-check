import {test} from 'node:test';
import assert from 'node:assert/strict';
import {discoverGateway} from './gateway-discovery.mjs';
test('loopback discovery verifies models and never uses plugin serverUrl',async()=>{
 let requested;const config=await discoverGateway({}, {MAGPIE_ADDR:'127.0.0.1:19999',XDG_CONFIG_HOME:'/missing'},async(url,init)=>{requested=url;assert.equal(init.headers.authorization,'Bearer magpie');return {ok:true,json:async()=>({data:[]})}});
 assert.equal(requested,'http://127.0.0.1:19999/v1/models');assert.equal(config.gateway_key,'magpie');
});
test('non-loopback automatic discovery and invalid responses do not persist credentials',async()=>{
 let calls=0;assert.equal(await discoverGateway({}, {MAGPIE_ADDR:'0.0.0.0:3425'},async()=>{calls++}),undefined);assert.equal(calls,0);
 assert.equal(await discoverGateway({gatewayURL:'http://127.0.0.1:3425'},{},async()=>({ok:false})),undefined);
});
