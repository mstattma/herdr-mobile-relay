<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte';
  import AttachmentProgress from '$components/AttachmentProgress.svelte';
  import Button from '$components/ui/Button.svelte';
  import { clearPromptDraft, loadPromptDraft, promptComposerScope, savePromptDraft } from '$lib/prompt-drafts';
  import { interfaceSize, promptMaxLines } from '$lib/preferences';
  import { resizePromptComposer } from '$lib/composer-sizing';
  import { openCodeCyclePrompt } from '$lib/opencode-agent-cycle';
  import { targetRefForAgent, targetRefMatchesAgent } from '$lib/resource-id';
  import { relayStore } from '$lib/store';
  import { securityState } from '$lib/security';
  import { terminalDraftState } from '$lib/terminal-draft-state.svelte';
  import { terminalInputDelivery } from '$lib/terminal-input-delivery.svelte';
  import { piPromptDelivery, piPromptTarget, validPiPromptText, type PiPromptCapability } from '$lib/pi-prompt.svelte';
  import { promptDispatchRestriction } from '$lib/terminal-input';
  import { freshAgentStatus, inputContextIdentity } from '$lib/input-target-state';
  import { guardReload, holdReloadsWhileSending } from '$lib/reload-guard';
  import { dispatchFlag, type ComposerSubmitAdapter } from '$lib/composer';
  import type { AttachmentBatchController, AttachmentBatchSnapshot, AttachmentRef } from '$lib/attachments';
  import type { Agent, SlashCommand, SlashCommandCatalog, TerminalFrame } from '$lib/types';

  // blockExplained: the parent already shows why sending is blocked, so the composer does not repeat dispatchBlocked.
  let { agent, frame, readOnly = false, locked = false, dispatchBlocked = '', blockExplained = false, attachmentsBlocked = false, draftPersistenceAllowed = false, noEchoActive = false, noEchoPrompt = 'Password:',
    placeholder = 'Type a reply…', submitLabel = 'Send prompt', submitAdapter, sending = $bindable(false), onsubmitted }:
    { agent: Agent; frame?: TerminalFrame; readOnly?: boolean; locked?: boolean; dispatchBlocked?: string; blockExplained?: boolean; attachmentsBlocked?: boolean; draftPersistenceAllowed?: boolean; noEchoActive?: boolean; noEchoPrompt?: string;
      placeholder?: string; submitLabel?: string; submitAdapter?: ComposerSubmitAdapter; sending?: boolean; onsubmitted?: () => void } = $props();
  const connections = relayStore.connections;
  const connection = $derived($connections.get(agent.relay_id));
  let mounted = true;
  let composerElement = $state<HTMLTextAreaElement>(null!);
  let fileInput = $state<HTMLInputElement>(null!);
  let imageInput = $state<HTMLInputElement>(null!);
  let composer = $state('');
  let draftTarget = untrack(() => promptComposerScope(agent));
  let draftRestored = false;
  let secretValue = $state('');
  const secretState = terminalDraftState(() => agent, () => frame);
  const openCode = $derived((agent.agent || '').toLowerCase().replace(/[ _-]/gu, '') === 'opencode');
  let cyclingAgent = $state(false);
  let cycledFrame = $state.raw<TerminalFrame>();
  const cycleReady = $derived(canCycleAgent());
  function canCycleAgent() {
    return openCode && !inputLocked && !sending && !sendingSecret && !uploadingAttachment
    && !cyclingAgent && !submitAdapter && visible && !$securityState.locked
    && freshAgentStatus(connection) && connection?.capabilities.includes('agent_cycle')
    && relayStore.deviceCredential(agent.relay_id)?.role === 'controller'
    && secretState.ready && frame?.noEcho === false && !frame.resizeSettling && frame !== cycledFrame
    && Boolean(frame.target && targetRefMatchesAgent(frame.target, agent))
    && openCodeCyclePrompt(frame.content) && !promptDispatchRestriction(agent)
    && (agent.status === 'idle' || agent.status === 'done' || agent.status === 'working' || agent.status === 'blocked' && agent.attention_kind === 'chat')
    && !agent.interaction && !agent.question_layout && !agent.options?.length
    && !['approval', 'question', 'unknown'].includes(agent.attention_kind || '');
  }
  const delivery = $derived(terminalInputDelivery(agent));
  const piDelivery = $derived(piPromptDelivery(agent));
  const deliveryBlocked = $derived(delivery.pending || delivery.uncertain || piDelivery.pending || piDelivery.uncertain);
  let piCapability = $state.raw<PiPromptCapability | null>(null);
  let piCapabilityTick = $state(0);
  const bridgeReady = $derived.by(() => {
    void piCapabilityTick;
    void connection;
    return !readOnly && !locked && !$securityState.locked && visible && !submitAdapter && !noEchoActive
      && !noEchoDraftTainted && draftPersistenceAllowed && dispatchBlocked === promptDispatchRestriction(agent)
      && relayStore.piPromptReady(agent, piCapability, frame);
  });
  const promptBlocked = $derived(Boolean(dispatchBlocked) && !bridgeReady);
  $effect.pre(() => {
    const state = delivery;
    const piState = piDelivery;
    state.retain();
    piState.retain();
    return () => { state.release(); piState.release(); };
  });
  let sendingSecret = $state(false);
  onMount(() => guardReload(() => (secretValue || composer && noEchoDraftTainted ? 'draft' : null)));
  let noEchoDraftBaseline: string | null = null;
  let noEchoDraftTainted = $state(false);
  let draftPersistenceWarning = $state('');
  let draftPersistenceError = $state(false);
  let visible = $state(document.visibilityState !== 'hidden');
  const slashCatalogIdentity = $derived(inputContextIdentity(agent));
  const piProbeKey = $derived(visible && !$securityState.locked && !readOnly && !locked && piPromptTarget(agent)
    && connection?.status === 'connected' && connection.capabilities.includes('pi_prompt_unconfirmed')
    ? slashCatalogIdentity : '');
  $effect(() => {
    const key = piProbeKey;
    piCapability = null;
    if (!key) return;
    let cancelled = false;
    const target = untrack(() => ({ ...agent }));
    const refresh = async () => {
      try {
        const capability = await relayStore.loadPiPromptCapability(target);
        if (!cancelled) { piCapability = capability; relayStore.readPane(target, true, 30); }
      } catch { if (!cancelled) piCapability = null; }
    };
    void untrack(refresh);
    const timer = setInterval(() => { piCapabilityTick += 1; void refresh(); }, 5_000);
    return () => { cancelled = true; clearInterval(timer); };
  });

  onMount(() => {
    const visibilityChanged = () => {
      visible = document.visibilityState !== 'hidden';
    };
    document.addEventListener('visibilitychange', visibilityChanged);
    return () => document.removeEventListener('visibilitychange', visibilityChanged);
  });

  let uploadStatus = $state('');
  let uploadError = $state(false);
  let uploadingAttachment = $state(false);
  let attachmentController = $state.raw<AttachmentBatchController | null>(null);
  let attachmentSnapshot = $state<AttachmentBatchSnapshot | null>(null);
  let attachmentTarget: Agent | null = null;
  let attachmentUnsubscribe: (() => void) | null = null;
  let attachmentCancelRequested = false;
  let attachmentGeneration = 0;
  let slashCatalog = $state<SlashCommandCatalog>({ commands: [], truncated: false });
  let slashCatalogLoading = $state(true);
  let slashCatalogUnavailable = $state(false);
  let slashCatalogRequest = 0;
  let slashCatalogTarget = '';
  let activeSlashIndex = $state(0);
  let dismissedSlashQuery = $state<string | null>(null);

  const MAX_VISIBLE_SLASH_COMMANDS = 200;
  const slashQuery = $derived(composer.startsWith('/') && !/\s/.test(composer) ? composer.slice(1).toLocaleLowerCase() : null);
  const matchingSlashCommands = $derived.by(() => {
    if (slashQuery === null) return [];
    if (!slashQuery) return slashCatalog.commands;
    return slashCatalog.commands.filter((entry) => entry.command.slice(1).toLocaleLowerCase().startsWith(slashQuery));
  });
  const filteredSlashCommands = $derived(matchingSlashCommands.slice(0, MAX_VISIBLE_SLASH_COMMANDS));
  const slashMatchesHidden = $derived(matchingSlashCommands.length > filteredSlashCommands.length);
  const effectiveSlashIndex = $derived(filteredSlashCommands.length
    ? Math.min(activeSlashIndex, filteredSlashCommands.length - 1)
    : -1);

  const composerLocked = $derived(readOnly || locked);
  const inputLocked = $derived(composerLocked || promptBlocked || attachmentsBlocked || noEchoActive || deliveryBlocked);
  const attachmentsLocked = $derived(inputLocked || piPromptTarget(agent) || sendingSecret || !targetRefForAgent(agent));
  const slashMenuOpen = $derived(!inputLocked && slashQuery !== null && dismissedSlashQuery !== composer);
  const secretMode = $derived(!readOnly && noEchoActive && Boolean(connection?.capabilities.includes('secret_input')));
  const secretReady = $derived(!deliveryBlocked && secretMode && secretState.secretReady);
  $effect(() => {
    if (!secretMode || !visible || $securityState.locked || deliveryBlocked || connection?.status !== 'connected') return;
    const target = untrack(() => ({ ...agent }));
    const refresh = () => relayStore.readPane(target, true, 30);
    refresh();
    const timer = setInterval(refresh, 2_000);
    return () => clearInterval(timer);
  });
  const NOT_PERSISTED_AT_HIDDEN_PROMPT =
    'Not saved on this phone: terminal input was hidden or could not be checked when this text was typed.';

  $effect.pre(() => {
    const identity = promptComposerScope(agent);
    if (identity === draftTarget) return;
    draftTarget = identity;
    composer = '';
    draftRestored = false;
    noEchoDraftBaseline = null;
    noEchoDraftTainted = false;
  });

  $effect(() => {
    draftPersistenceError = false;
    if (noEchoActive || !draftPersistenceAllowed) {
      if (noEchoDraftBaseline === null) noEchoDraftBaseline = composer;
      if (composer !== noEchoDraftBaseline) noEchoDraftTainted = true;
      draftPersistenceWarning = noEchoDraftTainted ? NOT_PERSISTED_AT_HIDDEN_PROMPT
        : !draftPersistenceAllowed ? 'Not saved on this phone: waiting for fresh terminal input state.' : '';
      return;
    }
    noEchoDraftBaseline = null;
    if (!draftRestored) {
      draftRestored = true;
      if (!composer && !noEchoDraftTainted) composer = untrack(() => loadPromptDraft(agent));
    }
    if (!composer) noEchoDraftTainted = false;
    if (noEchoDraftTainted) {
      draftPersistenceWarning = NOT_PERSISTED_AT_HIDDEN_PROMPT;
      return;
    }
    const result = savePromptDraft(agent, composer);
    draftPersistenceError = result === 'too-large' || result === 'unavailable';
    draftPersistenceWarning = result === 'too-large'
      ? 'This draft is too large to persist; it survives pane switches but not closing the app.'
      : result === 'unavailable'
        ? 'Draft persistence is unavailable; keep this page open.'
        : '';
  });

  $effect.pre(() => {
    const context = [slashCatalogIdentity, frame?.noEchoPrompt, secretReady];
    void context;
    untrack(() => { secretValue = ''; });
  });

  $effect(() => {
    if (!slashMenuOpen || effectiveSlashIndex < 0) return;
    const optionId = `slash-command-option-${effectiveSlashIndex}`;
    void tick().then(() => document.getElementById(optionId)?.scrollIntoView?.({ block: 'nearest' }));
  });

  $effect(() => {
    const resizeFor = [composer, $interfaceSize, $promptMaxLines];
    void tick().then(() => {
      if (mounted && resizeFor[0] === composer && resizeFor[1] === $interfaceSize && resizeFor[2] === $promptMaxLines) resizeComposer();
    });
  });

  onMount(() => {
    let pending = 0;
    const resize = () => {
      cancelAnimationFrame(pending);
      pending = requestAnimationFrame(resizeComposer);
    };
    let width = -1;
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => {
      const next = composerElement.getBoundingClientRect().width;
      if (next !== width) { width = next; resize(); }
    });
    observer?.observe(composerElement);
    const fonts = new MutationObserver(resize);
    fonts.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-interface-size'] });
    const viewport = window.visualViewport;
    viewport?.addEventListener('resize', resize);
    viewport?.addEventListener('scroll', resize);
    window.addEventListener('resize', resize);
    document.fonts?.addEventListener('loadingdone', resize);
    resize();
    return () => {
      cancelAnimationFrame(pending);
      observer?.disconnect();
      fonts.disconnect();
      viewport?.removeEventListener('resize', resize);
      viewport?.removeEventListener('scroll', resize);
      window.removeEventListener('resize', resize);
      document.fonts?.removeEventListener('loadingdone', resize);
    };
  });

  const slashConnectionStatus = $derived(connection?.status);

  async function refreshSlashCommands() {
    const request = ++slashCatalogRequest;
    const identity = slashCatalogIdentity;
    const target = { ...agent };
    slashCatalogLoading = true;
    slashCatalogUnavailable = false;
    try {
      const catalog = await relayStore.loadSlashCommands(target, true);
      if (request !== slashCatalogRequest || identity !== slashCatalogIdentity) return;
      slashCatalog = catalog;
    } catch {
      if (request !== slashCatalogRequest || identity !== slashCatalogIdentity) return;
      slashCatalogUnavailable = true;
    } finally {
      if (request === slashCatalogRequest && identity === slashCatalogIdentity) slashCatalogLoading = false;
    }
  }

  $effect(() => {
    const identity = slashCatalogIdentity;
    const status = slashConnectionStatus;
    const open = slashMenuOpen;
    untrack(() => {
      ++slashCatalogRequest;
      if (slashCatalogTarget !== identity) {
        slashCatalog = { commands: [], truncated: false };
        slashCatalogTarget = identity;
      }
      if (!open) return;
      if (status && status !== 'connected') {
        slashCatalogLoading = false;
        slashCatalogUnavailable = true;
        return;
      }
      void refreshSlashCommands();
    });
    return () => { ++slashCatalogRequest; };
  });

  async function sendPrompt() {
    const submittedDraft = composer;
    const adapter = submitAdapter;
    const text = adapter?.filter ? submittedDraft : submittedDraft.replace(/[\r\n]+$/g, '');
    if (!text || composerLocked || promptBlocked || deliveryBlocked || sending || uploadingAttachment || sendingSecret || noEchoActive) return;
    const bridge = Boolean(dispatchBlocked && bridgeReady);
    if (bridge && (!validPiPromptText(text) || !piCapability || !frame)) {
      relayStore.showToast('Pi prompts must be text only and at most 100,000 bytes.', true);
      return;
    }
    const validation = adapter?.validate?.(text);
    if (validation) {
      relayStore.showToast(validation, true);
      return;
    }
    const target = { ...agent };
    const identity = targetRefForAgent(target);
    if (!identity) {
      relayStore.showToast('This terminal does not have a stable input target.', true);
      return;
    }
    sending = true;
    composer = '';
    clearPromptDraft(target);
    const sent = holdReloadsWhileSending();
    try {
      if (bridge) await relayStore.sendPiPrompt(target, text, piCapability!, frame!);
      else if (adapter) await adapter.send(target, text);
      else await relayStore.sendToAgent(target, { type: 'submit_prompt', text });
      if (!bridge) relayStore.showToast(adapter?.sentMessage || 'Prompt sent.');
      if (mounted && targetRefMatchesAgent(identity, agent)) {
        uploadStatus = '';
        uploadError = false;
        onsubmitted?.();
      }
    } catch (error) {
      const unknown = dispatchFlag(error, 'dispatched_unknown');
      if (mounted && targetRefMatchesAgent(identity, agent) && !composer && !unknown
        && (!adapter?.filter || dispatchFlag(error, 'not_started'))) composer = submittedDraft;
      const detail = error instanceof Error ? error.message : adapter?.failureMessage || 'Prompt could not be sent.';
      if (!bridge || !unknown) relayStore.showToast(unknown ? `${detail} Check the terminal before sending again.` : detail, true);
    } finally {
      sent();
      sending = false;
      setTimeout(() => { if (mounted && targetRefMatchesAgent(identity, agent)) relayStore.readPane(target); }, 500);
    }
  }



  async function cycleAgent() {
    if (!canCycleAgent()) return;
    const target = { ...agent };
    const state = delivery;
    if (!state.begin()) return;
    cyclingAgent = true;
    cycledFrame = frame;
    const sent = holdReloadsWhileSending();
    let uncertain = true;
    try {
      const result = await relayStore.sendToAgent(target, { type: 'cycle_agent' });
      uncertain = result.phase !== 'sent' || result.data?.sent !== true || !result.ok;
      relayStore.showToast(uncertain ? 'Tab delivery is uncertain. Check Terminal before trying again.' : 'Tab sent to OpenCode. Check the terminal for the active agent.', uncertain);
    } catch (error) {
      uncertain = !dispatchFlag(error, 'not_started');
      const message = error instanceof Error ? error.message : 'Could not send Tab to OpenCode.';
      relayStore.showToast(uncertain ? `${message} Check Terminal before trying again.` : message, true);
    } finally {
      state.finish(uncertain);
      sent();
      cyclingAgent = false;
      if (mounted) relayStore.readPane(target, true, 30);
    }
  }

  async function submitSecret() {
    const secret = secretValue;
    if (!secret || !secretReady || !frame || dispatchBlocked || sendingSecret || uploadingAttachment || sending) return;
    const target = { ...agent };
    const identity = targetRefForAgent(target);
    if (!identity) return;
    sendingSecret = true;
    secretValue = '';
    try {
      await relayStore.sendSecret(target, secret, frame);
      relayStore.showToast('Password sent to the terminal.');
    } catch (error) {
      const message = error instanceof Error && error.message
        ? error.message
        : 'The password could not be sent.';
      relayStore.showToast(message, true);
    } finally {
      sendingSecret = false;
      setTimeout(() => { if (mounted && targetRefMatchesAgent(identity, agent)) relayStore.readPane(target); }, 500);
    }
  }

  function secretKeydown(event: KeyboardEvent) {
    if (event.isComposing || event.key !== 'Enter') return;
    event.preventDefault();
    void submitSecret();
  }

  function composerInput(event: Event) {
    draftRestored = true;
    if ((noEchoActive || !draftPersistenceAllowed) && (event.currentTarget as HTMLTextAreaElement).value !== noEchoDraftBaseline) noEchoDraftTainted = true;
    if (dismissedSlashQuery !== composer) dismissedSlashQuery = null;
    activeSlashIndex = 0;
  }

  function clearComposer() {
    draftRestored = true;
    noEchoDraftTainted = false;
    clearPromptDraft(agent);
    composer = '';
    uploadStatus = '';
    uploadError = false;
    dismissedSlashQuery = null;
    activeSlashIndex = 0;
  }

  function resizeComposer() {
    if (!composerElement) return;
    resizePromptComposer(composerElement, $promptMaxLines, window.visualViewport?.height ?? window.innerHeight);
  }

  async function selectSlashCommand(command: SlashCommand) {
    composer = `${command.command}${command.argument_hint ? ' ' : ''}`;
    dismissedSlashQuery = composer;
    activeSlashIndex = 0;
    await tick();
    composerElement.focus();
    composerElement.setSelectionRange(composer.length, composer.length);
  }

  function keydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      void sendPrompt();
      return;
    }
    if (!slashMenuOpen) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      dismissedSlashQuery = composer;
      return;
    }
    if (event.key === 'ArrowDown' && filteredSlashCommands.length) {
      event.preventDefault();
      activeSlashIndex = effectiveSlashIndex >= filteredSlashCommands.length - 1 ? 0 : effectiveSlashIndex + 1;
      return;
    }
    if (event.key === 'ArrowUp' && filteredSlashCommands.length) {
      event.preventDefault();
      activeSlashIndex = effectiveSlashIndex <= 0 ? filteredSlashCommands.length - 1 : effectiveSlashIndex - 1;
      return;
    }
    if ((event.key === 'Enter' || event.key === 'Tab') && effectiveSlashIndex >= 0) {
      event.preventDefault();
      void selectSlashCommand(filteredSlashCommands[effectiveSlashIndex]);
    }
  }

  function appendUploadedAttachments(attachments: AttachmentRef[]): void {
    const rejected = attachmentSnapshot?.items.filter((item) => item.state === 'rejected') || [];
    if (!attachments.length) {
      // A current cancel failure takes precedence over the canceled upload's completion.
      if (attachmentCancelRequested && uploadError) return;
      uploadStatus = attachmentCancelRequested
        ? 'Attachment upload canceled.'
        : rejected.length
          ? 'No selected attachments passed validation.'
          : 'No attachments were uploaded.';
      uploadError = !attachmentCancelRequested;
      return;
    }
    const prefix = composer && !composer.endsWith('\n') ? '\n' : '';
    composer += `${prefix}${attachments.map((attachment) => `Attachment: ${attachment.ref}`).join('\n')}\n`;
    uploadStatus = `Attached ${attachments.map((attachment) => attachment.name).join(', ')}${rejected.length ? `; ${rejected.length} rejected` : ''}`;
    uploadError = rejected.length > 0;
    if (!rejected.length) attachmentSnapshot = null;
  }

  function releaseAttachmentController(controller: AttachmentBatchController, force = false): void {
    if (attachmentController !== controller) return;
    if (!force && attachmentSnapshot?.items.some((item) => item.state === 'interrupted')) return;
    attachmentUnsubscribe?.();
    attachmentUnsubscribe = null;
    attachmentController = null;
  }

  async function filesSelected(files: FileList | File[]) {
    const selected = [...files];
    if (attachmentsLocked || !selected.length || uploadingAttachment || sending) return;
    attachmentGeneration += 1;
    const target = { ...agent };
    const identity = targetRefForAgent(target)!;
    uploadingAttachment = true;
    uploadStatus = `Uploading ${selected.length} attachment${selected.length === 1 ? '' : 's'}…`;
    uploadError = false;
    attachmentCancelRequested = false;
    let controller: AttachmentBatchController | null = null;
    try {
      const previous = attachmentController;
      if (previous) {
        try {
          await previous.cancel();
        } finally {
          releaseAttachmentController(previous, true);
        }
      }
      controller = relayStore.attachmentController(target);
      attachmentTarget = target;
      attachmentController = controller;
      attachmentUnsubscribe?.();
      attachmentUnsubscribe = controller.subscribe((snapshot) => {
        attachmentSnapshot = snapshot;
      });
      controller.select(selected);
      const attachments = await controller.upload();
      if (mounted && targetRefMatchesAgent(identity, agent) && !readOnly) appendUploadedAttachments(attachments);
    } catch (error) {
      uploadStatus = attachmentCancelRequested
        ? 'Attachment upload canceled.'
        : error instanceof Error && error.message
          ? error.message
          : 'Attachments could not be uploaded.';
      uploadError = !attachmentCancelRequested;
    } finally {
      uploadingAttachment = false;
      if (controller) releaseAttachmentController(controller);
    }
  }
  async function restartAttachmentUpload(): Promise<void> {
    const controller = attachmentController;
    const identity = attachmentTarget && targetRefForAgent(attachmentTarget);
    if (attachmentsLocked || !controller || uploadingAttachment || sending || !identity || !targetRefMatchesAgent(identity, agent)) return;
    attachmentGeneration += 1;
    uploadingAttachment = true;
    uploadStatus = 'Restarting interrupted files from the beginning…';
    uploadError = false;
    attachmentCancelRequested = false;
    try {
      const attachments = await controller.restart();
      if (mounted && targetRefMatchesAgent(identity, agent) && !readOnly) appendUploadedAttachments(attachments);
    } catch (error) {
      uploadStatus = error instanceof Error && error.message
        ? error.message
        : 'Attachments could not be restarted.';
      uploadError = true;
    } finally {
      uploadingAttachment = false;
      releaseAttachmentController(controller);
    }
  }

  async function cancelAttachmentUpload(): Promise<void> {
    const controller = attachmentController;
    if (!controller) return;
    const generation = attachmentGeneration;
    attachmentCancelRequested = true;
    try {
      await controller.cancel();
      if (generation === attachmentGeneration) attachmentSnapshot = null;
    } catch {
      if (generation === attachmentGeneration) {
        uploadStatus = 'The relay could not confirm attachment cancellation.';
        uploadError = true;
      }
    } finally {
      releaseAttachmentController(controller, true);
    }
  }

  onDestroy(() => {
    mounted = false;
    attachmentUnsubscribe?.();
    void attachmentController?.cancel().catch(() => relayStore.showToast('The relay could not confirm attachment cancellation.', true));
  });

  function paste(event: ClipboardEvent) {
    const files = [...(event.clipboardData?.items || [])]
      .filter((item) => item.kind === 'file')
      .map((item) => item.getAsFile())
      .filter((file): file is File => Boolean(file));
    if (!files.length) return;
    event.preventDefault();
    void filesSelected(files);
  }

</script>
<div class="prompt-composer">
    {#if slashMenuOpen}
      <section class="slash-command-popover" aria-label="Command suggestions">
        <header class="slash-command-header">
          <strong>Commands</strong>
          {#if !slashCatalogLoading && !slashCatalogUnavailable}
            <span>{filteredSlashCommands.length}{slashMatchesHidden ? '+' : ''} matching</span>
          {:else}
            <span>Type to filter</span>
          {/if}
        </header>
        {#if slashCatalog.status === 'partial'}
          <p class="slash-command-status" role="status">Runtime command discovery is incomplete. Loaded extensions and prompts may be missing; you can still send a command manually.</p>
        {:else if slashCatalog.status === 'loading'}
          <p class="slash-command-status" role="status">Pi is loading command resources. Refresh when loading finishes.</p>
        {:else if slashCatalog.status === 'unavailable'}
          <p class="slash-command-status" role="status">Runtime command discovery is unavailable. You can still send a command manually.</p>
        {/if}
        <button type="button" onclick={() => void refreshSlashCommands()} disabled={slashCatalogLoading}>
          Refresh commands
        </button>
        {#if slashCatalogLoading}
          <p class="slash-command-status" role="status">{slashCatalog.commands.length ? 'Refreshing commands…' : 'Loading commands…'}</p>
        {:else if slashCatalogUnavailable}
          <p class="slash-command-status" role="status">Suggestions unavailable — you can still send this command.</p>
        {:else if !filteredSlashCommands.length}
          <p class="slash-command-status" role="status">No matching command — you can still send it.</p>
        {/if}
        <div
          id="slash-command-options"
          class="slash-command-menu"
          role="listbox"
          aria-label="Slash commands"
          aria-busy={slashCatalogLoading}
        >
          {#each filteredSlashCommands as entry, index (entry.command)}
            <button
              id={`slash-command-option-${index}`}
              type="button"
              role="option"
              tabindex="-1"
              class:active={index === effectiveSlashIndex}
              aria-selected={index === effectiveSlashIndex}
              onpointerdown={(event) => event.preventDefault()}
              onpointerenter={() => { activeSlashIndex = index; }}
              onclick={() => selectSlashCommand(entry)}
            >
              <span class="slash-command-name">
                <strong>{entry.command}</strong>
                {#if entry.argument_hint}<small>{entry.argument_hint}</small>{/if}
              </span>
              <span class="slash-command-description">{entry.description}</span>
              {#if entry.source !== 'builtin'}<em class="slash-command-source">{entry.source}</em>{/if}
            </button>
          {/each}
        </div>
        {#if !slashCatalogLoading && slashCatalog.truncated}
          <p class="slash-command-limit" role="status">Command suggestions may be incomplete because a discovery limit was reached. Typing searches only loaded suggestions; you can still send a command manually.</p>
        {/if}
        {#if !slashCatalogLoading && slashMatchesHidden}
          <p class="slash-command-display-limit" role="status">More matching commands are hidden; keep typing to narrow the list.</p>
        {/if}
      </section>
    {/if}
    {#if noEchoActive}
      <section class="secret-prompt" aria-label="Hidden terminal prompt">
        <p id="secret-prompt-line" role="status">
          The terminal is asking for a hidden value: <strong>{noEchoPrompt}</strong>
        </p>
        {#if connection?.capabilities.includes('secret_input')}
          <div class="secret-prompt-row">
            <input
              bind:value={secretValue}
              type="password"
              aria-label="Value for the hidden terminal prompt"
              aria-describedby="secret-prompt-line"
              autocomplete="off"
              autocapitalize="none"
              spellcheck="false"
              enterkeyhint="send"
              disabled={!secretReady || sendingSecret || sending || uploadingAttachment}
              onkeydown={secretKeydown}
            />
            <Button
              size="sm"
              disabled={!secretReady || Boolean(dispatchBlocked) || !secretValue || sendingSecret || sending || uploadingAttachment}
              aria-label="Send hidden value"
              onclick={submitSecret}
            >{sendingSecret ? '…' : 'Send'}</Button>
          </div>
          <p class="hint">Typed straight into the terminal: never saved on this phone and never written to activity.</p>
          {#if !secretReady}<p role="status">Wait for fresh terminal context, or inspect uncertain delivery in Terminal.</p>{/if}
        {:else}
          <p class="hint">This relay does not support hidden input; answer at the computer.</p>
        {/if}
      </section>
    {/if}
    <form class="term-input" aria-label="Send a prompt" aria-busy={sending || uploadingAttachment} onsubmit={(event) => { event.preventDefault(); void sendPrompt(); }}>
      <div class="attach-stack">
      <Button variant="ghost" size="icon" disabled={attachmentsLocked || uploadingAttachment || sending} aria-label="Attach photos" onclick={() => imageInput.click()}>
        <svg class="button-symbol" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
          <rect x="3" y="4" width="18" height="16" rx="2"></rect>
          <circle cx="8.5" cy="9" r="1.5"></circle>
          <path d="m4 17 4.5-4.5 3.5 3.5 2.5-2.5L20 19"></path>
        </svg>
      </Button>
      <Button variant="ghost" size="icon" disabled={attachmentsLocked || uploadingAttachment || sending} aria-label="Attach files" onclick={() => fileInput.click()}>
        <svg class="button-symbol" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
          <path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"></path>
        </svg>
      </Button>
      </div>
      <div class:has-text={Boolean(composer)} class="composer-field">
        <textarea
          bind:this={composerElement}
          bind:value={composer}
          rows="1"
          disabled={composerLocked}
          placeholder={placeholder}
          role="combobox"
          aria-label="Prompt"
          aria-autocomplete="list"
          aria-haspopup="listbox"
          aria-expanded={slashMenuOpen}
          aria-controls={slashMenuOpen ? 'slash-command-options' : undefined}
          aria-activedescendant={slashMenuOpen && effectiveSlashIndex >= 0 ? `slash-command-option-${effectiveSlashIndex}` : undefined}
          autocomplete="off"
          autocorrect="on"
          autocapitalize="sentences"
          spellcheck="true"
          enterkeyhint="enter"
          oninput={composerInput}
          onkeydown={keydown}
          onpaste={paste}
        ></textarea>
        {#if composer}<button type="button" class="input-clear" aria-label="Clear prompt text" onclick={clearComposer}>×</button>{/if}
      </div>
      <div class="submit-stack">
        {#if openCode}
          <Button variant="secondary" size="sm" class="cycle-agent" disabled={!cycleReady} aria-label="Switch agent"
            title="Sends the default Tab shortcut to cycle OpenCode primary agents (Build, Plan, or others). Custom keybindings are not supported; this does not select or confirm a mode."
            onclick={cycleAgent}>Switch<br />agent</Button>
        {/if}
        <Button type="submit" size="icon" disabled={!composer.replace(/[\r\n]+$/g, '') || composerLocked || promptBlocked || deliveryBlocked || noEchoActive || sending || uploadingAttachment || !targetRefForAgent(agent)} aria-label={sending ? 'Submitting input' : submitLabel} >{sending ? '…' : '➤'}</Button>
      </div>
      <input bind:this={imageInput} type="file" accept="image/*" multiple hidden onchange={(event) => { void filesSelected(event.currentTarget.files || []); event.currentTarget.value = ''; }} />
      <input bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp,text/plain,text/markdown,text/csv,application/json,application/pdf,.docx,.xlsx,.pptx,.odt,.ods,.odp" multiple hidden onchange={(event) => { void filesSelected(event.currentTarget.files || []); event.currentTarget.value = ''; }} />
    </form>
    {#if !readOnly && attachmentSnapshot?.items.length}
      <AttachmentProgress snapshot={attachmentSnapshot} restartDisabled={attachmentsLocked || sending} oncancel={cancelAttachmentUpload} onrestart={restartAttachmentUpload} />
    {/if}
    {#if !readOnly && piDelivery.uncertain}
      <div class="hint" role="status">
        <span>{piDelivery.error || (piDelivery.handed ? 'Sent to Pi — not confirmed' : 'Pi send not confirmed')}</span>
        <button type="button" disabled={composerLocked || $securityState.locked} onclick={() => piDelivery.acknowledge()}>I checked — allow another send</button>
      </div>
    {:else if bridgeReady && !piDelivery.pending}<p class="hint">Pi bridge: unconfirmed send, no terminal keys.</p>
    {:else if dispatchBlocked && !blockExplained && !readOnly && !piDelivery.pending}
      <p class="hint" role="status">{piPromptTarget(agent) && !piCapability ? 'For live Pi replies, enable Settings → Pi Integration, then /reload Pi.' : dispatchBlocked}</p>
    {/if}
    {#if uploadStatus}<p class:error={uploadError} class="upload-status" role="status">{uploadStatus}</p>{/if}
    {#if draftPersistenceWarning && (composer || draftPersistenceError)}<p class="draft-status" class:error={draftPersistenceError} role="status">{draftPersistenceWarning}</p>{/if}

  {#if !readOnly && !targetRefForAgent(agent)}
    <p class="upload-status error" role="status">Attachments and sending need a stable target. Wait for the agent inventory to refresh.</p>
  {/if}
</div>

<style>
  .submit-stack { align-items: center; display: flex; flex-direction: column; gap: .3rem; }
  .submit-stack :global(.cycle-agent) { font-size: .65rem; line-height: 1.2; padding: .3rem; }
  .composer-field textarea { overscroll-behavior: contain; }
</style>
