<script lang="ts">
  /*
   * The whole page. There is no router: floe is one page with one pair open at
   * a time, and which pair that is lives in the URL's fragment beside the
   * token, so a reload lands back where it was.
   */
  import { Api, ApiError } from './lib/api'
  import { readSession, writePair } from './lib/session'
  import Blocked from './lib/icons/Blocked.svelte'
  import MainScreen from './screens/Main.svelte'
  import PairsScreen from './screens/Pairs.svelte'
  import SettingsScreen from './screens/Settings.svelte'
  import TopBar from './lib/TopBar.svelte'
  import type { Pair, Pairs } from './lib/types'

  const session = readSession()
  const api = new Api(session.token)

  let pairs = $state<Pairs | undefined>(undefined)
  let open = $state<Pair | undefined>(undefined)
  /*
   * Screen 3 sits in front of the open pair rather than beside it: its
   * patterns decide what crosses, so leaving it puts the transfer screens back
   * with the pair as it now stands.
   */
  let settings = $state(false)
  let failure = $state<string>('')

  // The token reaches the page in the fragment; without it every API request
  // is refused, so say so rather than showing an empty list.
  if (!session.token) {
    failure = 'This page was opened without floe’s token. Open floe at the address it printed.'
  } else {
    void start()
  }

  async function start() {
    try {
      pairs = await api.pairs()
      if (session.pair) await openPair(session.pair)
    } catch (e) {
      failure = message(e)
    }
  }

  async function openPair(id: string) {
    try {
      open = await api.open(id)
      settings = false
      writePair(id)
    } catch (e) {
      // A pair floe cannot open is not a dead end: the list still stands.
      open = undefined
      writePair('')
      failure = message(e)
    }
  }

  function toPairs() {
    open = undefined
    settings = false
    failure = ''
    writePair('')
    void refresh()
  }

  async function refresh() {
    try {
      pairs = await api.pairs()
    } catch (e) {
      failure = message(e)
    }
  }

  function message(e: unknown): string {
    return e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e)
  }
</script>

{#if open && settings}
  <SettingsScreen
    {api}
    pair={open}
    home={pairs?.home}
    onback={() => (settings = false)}
    onsaved={(saved) => {
      open = saved
      settings = false
    }}
  />
{:else if open}
  <MainScreen
    {api}
    pair={open}
    home={pairs?.home}
    onpairs={toPairs}
    onsettings={() => (settings = true)}
    onpair={(fresh) => (open = fresh)}
  />
{:else if pairs}
  {#if failure}
    <div class="banner">
      <Blocked />
      <span>{failure}</span>
      <button type="button" onclick={() => (failure = '')}>Dismiss</button>
    </div>
  {/if}
  <PairsScreen {pairs} onopen={openPair} />
{:else if failure}
  <div class="screen">
    <TopBar />
    <main>
      <span class="line">
        <Blocked />
        <span>{failure}</span>
      </span>
    </main>
  </div>
{/if}

<style>
  .screen {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  main {
    width: 820px;
    align-self: center;
    padding-top: 64px;
  }

  .line {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--refusal);
  }

  .banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 18px;
    background: rgba(224, 130, 111, 0.1);
    border-bottom: 1px solid rgba(224, 130, 111, 0.3);
    color: var(--refusal);
  }

  .banner button {
    margin-left: auto;
    padding: 4px 10px;
    border: 1px solid var(--control);
    border-radius: 4px;
    background: none;
    font: inherit;
    color: var(--text);
    cursor: pointer;
  }
</style>
