import { n as noop } from './utils2-UOC6HMdI.js';
import './exports-UPa9cz8i.js';

const is_legacy = noop.toString().includes("$$") || /function \w+\(\) \{\}/.test(noop.toString());
if (is_legacy) {
  ({
    url: new URL("https://example.com")
  });
}
//# sourceMappingURL=state.svelte-BlSQeTLt.js.map
