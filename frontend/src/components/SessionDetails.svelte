<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import AppDialog from '$components/ui/AppDialog.svelte';
  import Button from '$components/ui/Button.svelte';
  import { targetRefForAgent } from '$lib/resource-id';
  import { securityState } from '$lib/security';
  import { copySessionField, loadSessionDetails, type SessionDetailsData } from '$lib/session-details';
  import { sessionTitle } from '$lib/session-overview';
  import { relayStore } from '$lib/store';
  import type { Agent } from '$lib/types';

  let { open = $bindable(false), agent, returnFocus = null }: { open?: boolean; agent: Agent | null; returnFocus?: HTMLElement | null } = $props();
  const connections = relayStore.connections;
  const identity = $derived(agent ? JSON.stringify([agent.pane_id, targetRefForAgent(agent)]) : '');
  const connection = $derived(agent ? $connections.get(agent.relay_id) : undefined);
  const ready = $derived(connection?.status === 'connected' && connection.inventory.state === 'ready');
  const supported = $derived(connection?.capabilities.includes('session_details') === true);
  let openedIdentity = $state('');
  let data = $state<SessionDetailsData | null>(null);
  let error = $state('');
  let loading = $state(false);
  let copyStatus = $state('');
  let idInput = $state<HTMLInputElement>();
  let visible = $state(true);
  const observedAt = $derived.by(() => {
    const value = data?.subscriptions?.observed_at;
    if (value == null) return null;
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? date : null;
  });

  onMount(() => {
    const changed = () => { visible = document.visibilityState !== 'hidden'; };
    changed();
    document.addEventListener('visibilitychange', changed);
    return () => document.removeEventListener('visibilitychange', changed);
  });

  $effect(() => {
    if (!open) { openedIdentity = ''; return; }
    if ($securityState.locked || !agent || (openedIdentity && openedIdentity !== identity)) {
      open = false;
      return;
    }
    openedIdentity = identity;
  });

  $effect(() => {
    if (!open) return;
    // A touch click need not focus its button, and lazy loading may finish later.
    const trigger = untrack(() => returnFocus) || document.activeElement;
    let disposed = false;
    let dialog: HTMLDialogElement | null = null;
    const trapTab = (event: KeyboardEvent) => {
      if (event.key !== 'Tab' || event.altKey || event.ctrlKey || event.metaKey
        || get(securityState).locked || !dialog?.open) return;
      const controls = [...dialog.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)')]
        .filter((element) => element.tabIndex >= 0);
      const first = controls[0];
      const last = controls.at(-1);
      const active = document.activeElement;
      if (active !== dialog && active !== (event.shiftKey ? first : last)) return;
      const destination = event.shiftKey ? last : first;
      if (!destination) return;
      event.preventDefault();
      destination.focus();
    };
    void tick().then(() => {
      if (disposed || get(securityState).locked) return;
      dialog = document.querySelector<HTMLDialogElement>('#session-details');
      dialog?.addEventListener('keydown', trapTab);
      dialog?.querySelector<HTMLButtonElement>('#session-details-close')?.focus();
    });
    return () => {
      disposed = true;
      dialog?.removeEventListener('keydown', trapTab);
      void tick().then(() => {
        if (!get(securityState).locked && trigger instanceof HTMLElement && trigger.isConnected) trigger.focus();
      });
    };
  });

  $effect(() => {
    void identity;
    data = null;
    error = '';
    copyStatus = '';
    loading = false;
    if (!open || $securityState.locked || !ready || !supported || !visible) return;
    const target = untrack(() => agent ? { ...agent } : null);
    if (!target) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = async () => {
      loading = true;
      try {
        const result = await loadSessionDetails(target, controller.signal);
        if (!controller.signal.aborted) { data = result; error = ''; }
      } catch (caught) {
        if (!controller.signal.aborted) {
          data = null;
          error = caught instanceof Error ? caught.message : 'Session details are unavailable.';
        }
      } finally {
        if (!controller.signal.aborted) {
          loading = false;
          timer = setTimeout(() => { void refresh(); }, 15_000);
        }
      }
    };
    void refresh();
    return () => { controller.abort(); clearTimeout(timer); };
  });

  async function copyId() {
    if (!idInput || $securityState.locked) return;
    const copiedIdentity = identity;
    const copied = await copySessionField(idInput);
    if (open && identity === copiedIdentity && !$securityState.locked) {
      copyStatus = copied ? 'Session ID copied.' : 'Session ID selected. Use your device\'s Copy command.';
    }
  }
</script>

{#if !$securityState.locked && agent}
  <AppDialog id="session-details" bind:open title="Session details">
    <div class="details-close"><Button id="session-details-close" variant="secondary" size="sm" onclick={() => { open = false; }}>Close</Button></div>
    <dl class="details-fields">
      <dt>Title</dt><dd>{data?.title ? sessionTitle({ ...agent, session_name: data.title }) : sessionTitle(agent)}</dd>
      <dt>Working directory</dt><dd class="details-path">{data?.cwd || agent.cwd || 'Not reported'}</dd>
      <dt>Status / agent</dt><dd>{agent.status || 'Unknown'} / {agent.agent || 'Unknown'}</dd>
    </dl>
    <label class="details-label" for="session-details-id">Native session ID</label>
    {#if agent.agent_session_id}
      <div class="details-id">
        <input id="session-details-id" bind:this={idInput} readonly value={agent.agent_session_id} spellcheck="false" />
        <Button variant="secondary" size="sm" onclick={copyId} aria-label="Copy session ID">Copy</Button>
      </div>
    {:else}
      <p>Not created yet</p>
    {/if}
    <p class="details-note" role="status">{copyStatus}</p>
    {#if !ready}<p class="details-note">Session details are unavailable while the relay reconnects or inventory recovers.</p>
    {:else if !supported}<p class="details-note">This relay does not provide session details.</p>{/if}
    {#if error}<p class="details-note" role="status">{error}</p>{/if}
    {#if loading && !data}<p class="details-note" role="status">Loading session details...</p>{/if}
    {#if data?.reason}<p class="details-note">Details status: {data.reason.replaceAll('_', ' ')}.</p>{/if}
    <section aria-labelledby="session-details-todos">
      <h3 id="session-details-todos">Todos</h3>
      {#if data?.todos_available}
        <ul class="details-rows">
          {#each data.todos as todo, index (index)}
            <li><span>{todo.content}</span><small>{todo.status.replaceAll('_', ' ')}{todo.priority ? ` / ${todo.priority}` : ''}</small></li>
          {:else}<li>No todos.</li>{/each}
        </ul>
      {:else}<p class="details-note">Todos unavailable.</p>{/if}
    </section>
    <section aria-labelledby="session-details-subscriptions">
      <h3 id="session-details-subscriptions">Subscriptions</h3>
      <p class="details-note">Usage is account-wide, not per session. Current marks the account selected for this session when reported.</p>
      {#if data?.subscriptions?.available}
        <ul class="details-rows">
          {#each data.subscriptions.rows as row, index (index)}
            <li>
              <div class="subscription-title"><strong>{row.label || row.provider}</strong>{#if row.current}<span class="current-marker">Current</span>{/if}</div>
              <small>{row.provider}{row.status ? ` / ${row.status}` : ''}</small>
              {#if row.paid}<small>{row.paid}</small>{/if}
              {#if row.detail}<span>{row.detail}</span>{/if}
              {#if row.limits}<small>{row.limits}</small>{/if}
              {#if row.weeklyReset}<small>Weekly reset: {row.weeklyReset}</small>{/if}
            </li>
          {:else}<li>No subscription observations.</li>{/each}
        </ul>
        {#if observedAt}
          <p class="details-note">Last observation: <time datetime={observedAt.toISOString()}>{observedAt.toLocaleString()}</time></p>
        {/if}
      {:else}<p class="details-note">Subscription observations unavailable.</p>{/if}
      <p class="details-note">Shows existing observations only; opening details does not refresh account tokens.</p>
    </section>
    <p class="details-note">Conversation branches describe conversation undo and fork history, not Git snapshots. Saved history does not by itself verify the active conversation branch.</p>
  </AppDialog>
{/if}

<style>
  :global(#session-details .dialog-content) { border-radius: .9rem 0 0 .9rem; bottom: 0; height: 100%; left: auto; max-height: 100dvh; max-width: 28rem; padding: max(1rem, env(safe-area-inset-top)) max(1rem, env(safe-area-inset-right)) max(1rem, env(safe-area-inset-bottom)) 1rem; right: 0; top: 0; transform: none; width: 100%; }
  .details-close { display: flex; justify-content: flex-end; margin-top: .5rem; }
  .details-fields { display: grid; gap: .3rem; margin: .75rem 0; }
  .details-fields dt, .details-label { color: var(--muted); font-size: .75rem; }
  .details-fields dd { margin: 0 0 .5rem; overflow-wrap: anywhere; user-select: text; }
  .details-path { font-size: .85rem; }
  .details-id { display: flex; gap: .4rem; margin-top: .3rem; }
  .details-id input { flex: 1; font-family: monospace; min-width: 0; user-select: text; width: 0; }
  .details-note { color: var(--muted); font-size: .75rem; line-height: 1.5; overflow-wrap: anywhere; }
  h3 { font-size: .85rem; margin: 1rem 0 .5rem; }
  .details-rows { list-style: none; margin: 0; padding: 0; }
  .details-rows li { border-bottom: 1px solid var(--border); display: grid; font-size: .8rem; gap: .25rem; overflow-wrap: anywhere; padding: .5rem 0; }
  .details-rows small { color: var(--muted); }
  .subscription-title { align-items: baseline; display: flex; flex-wrap: wrap; gap: .5rem; }
  .current-marker { border: 1px solid var(--border); border-radius: .25rem; font-size: .7rem; padding: .1rem .3rem; }
  @media (max-width: 30rem) { :global(#session-details .dialog-content) { border-radius: 0; } }
</style>
