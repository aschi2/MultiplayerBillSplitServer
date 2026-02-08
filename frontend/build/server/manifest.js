const manifest = (() => {
function __memo(fn) {
	let value;
	return () => value ??= (value = fn());
}

return {
	appDir: "_app",
	appPath: "_app",
	assets: new Set([]),
	mimeTypes: {},
	_: {
		client: {start:"_app/immutable/entry/start.Bao_Xww9.js",app:"_app/immutable/entry/app.B4DZblVf.js",imports:["_app/immutable/entry/start.Bao_Xww9.js","_app/immutable/chunks/DNDe-fJV.js","_app/immutable/chunks/yo6EL1wl.js","_app/immutable/chunks/CMfPz1dr.js","_app/immutable/entry/app.B4DZblVf.js","_app/immutable/chunks/yo6EL1wl.js","_app/immutable/chunks/LBoZHYo6.js","_app/immutable/chunks/BETbJ1jZ.js","_app/immutable/chunks/c0rXTrr9.js","_app/immutable/chunks/CMfPz1dr.js","_app/immutable/chunks/B0Lk4f_2.js","_app/immutable/chunks/CdcQO1R9.js"],stylesheets:[],fonts:[],uses_env_dynamic_public:false},
		nodes: [
			__memo(() => import('./chunks/0-ULjX_PoC.js')),
			__memo(() => import('./chunks/1-DJ59nzWb.js')),
			__memo(() => import('./chunks/2-nj0oGkzr.js')),
			__memo(() => import('./chunks/3-Cej9sRfE.js'))
		],
		remotes: {
			
		},
		routes: [
			{
				id: "/",
				pattern: /^\/$/,
				params: [],
				page: { layouts: [0,], errors: [1,], leaf: 2 },
				endpoint: null
			},
			{
				id: "/room/[roomCode]",
				pattern: /^\/room\/([^/]+?)\/?$/,
				params: [{"name":"roomCode","optional":false,"rest":false,"chained":false}],
				page: { layouts: [0,], errors: [1,], leaf: 3 },
				endpoint: null
			}
		],
		prerendered_routes: new Set([]),
		matchers: async () => {
			
			return {  };
		},
		server_assets: {}
	}
}
})();

const prerendered = new Set([]);

const base = "";

export { base, manifest, prerendered };
//# sourceMappingURL=manifest.js.map
