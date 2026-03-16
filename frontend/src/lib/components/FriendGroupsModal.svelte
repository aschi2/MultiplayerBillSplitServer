<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import {
    loadFriendGroups,
    saveFriendGroups,
    generateFriendGroupId,
    type FriendGroup,
    type FriendGroupMember
  } from '$lib/friendGroups';

  export let open = false;
  export let initialGroupId = '';

  const dispatch = createEventDispatcher<{
    close: void;
    groupschange: { groups: FriendGroup[]; selectedGroupId: string };
  }>();

  let friendGroups: FriendGroup[] = [];
  let friendGroupError: string | null = null;
  let friendGroupEditingId = '';
  let friendGroupDraftName = '';
  let friendGroupDraftMembers: Array<{ name: string; venmoUsername: string }> = [
    { name: '', venmoUsername: '' }
  ];
  let wasOpen = false;

  const normalizeVenmoUsername = (value: string | null | undefined) =>
    (value || '')
      .trim()
      .replace(/^@+/, '')
      .replace(/\s+/g, '');

  const loadFriendGroupsIntoState = () => {
    friendGroups = loadFriendGroups();
  };

  const emitGroupsChange = () => {
    dispatch('groupschange', { groups: friendGroups, selectedGroupId: friendGroupEditingId });
  };

  const resetFriendGroupDraft = () => {
    friendGroupEditingId = '';
    friendGroupDraftName = '';
    friendGroupDraftMembers = [{ name: '', venmoUsername: '' }];
    friendGroupError = null;
  };

  const openFriendGroupEditor = (groupId = '') => {
    loadFriendGroupsIntoState();
    friendGroupError = null;
    if (!groupId) {
      resetFriendGroupDraft();
      return;
    }
    const group = friendGroups.find((candidate) => candidate.id === groupId);
    if (!group) {
      resetFriendGroupDraft();
      return;
    }
    friendGroupEditingId = group.id;
    friendGroupDraftName = group.name;
    friendGroupDraftMembers = (group.members || []).map((member) => ({
      name: member.name,
      venmoUsername: normalizeVenmoUsername(member.venmoUsername || '')
    }));
    if (friendGroupDraftMembers.length === 0) {
      friendGroupDraftMembers = [{ name: '', venmoUsername: '' }];
    }
  };

  const addFriendGroupDraftMember = () => {
    friendGroupDraftMembers = [...friendGroupDraftMembers, { name: '', venmoUsername: '' }];
  };

  const removeFriendGroupDraftMember = (index: number) => {
    friendGroupDraftMembers = friendGroupDraftMembers.filter((_, idx) => idx !== index);
    if (friendGroupDraftMembers.length === 0) {
      friendGroupDraftMembers = [{ name: '', venmoUsername: '' }];
    }
  };

  const normalizeFriendGroupDraftMembers = () => {
    const deduped = new Map<string, FriendGroupMember>();
    friendGroupDraftMembers.forEach((member) => {
      const name = `${member?.name || ''}`.trim();
      if (!name) return;
      const key = name.toLowerCase();
      const venmoUsername = normalizeVenmoUsername(member?.venmoUsername || '');
      deduped.set(key, { name, venmoUsername: venmoUsername || undefined });
    });
    return Array.from(deduped.values()).slice(0, 32);
  };

  const saveFriendGroupDraftAsNew = () => {
    const groupName = friendGroupDraftName.trim();
    const members = normalizeFriendGroupDraftMembers();
    if (!groupName) {
      friendGroupError = 'Group name is required.';
      return;
    }
    if (!members.length) {
      friendGroupError = 'Add at least one person.';
      return;
    }
    loadFriendGroupsIntoState();
    const next: FriendGroup = {
      id: generateFriendGroupId(),
      name: groupName,
      members,
      createdAt: Date.now(),
      updatedAt: Date.now()
    };
    const merged = [next, ...friendGroups].slice(0, 24);
    saveFriendGroups(merged);
    loadFriendGroupsIntoState();
    friendGroupEditingId = next.id;
    friendGroupError = null;
    emitGroupsChange();
  };

  const updateFriendGroupDraft = () => {
    if (!friendGroupEditingId) {
      friendGroupError = 'Choose a group to update first.';
      return;
    }
    const groupName = friendGroupDraftName.trim();
    const members = normalizeFriendGroupDraftMembers();
    if (!groupName) {
      friendGroupError = 'Group name is required.';
      return;
    }
    if (!members.length) {
      friendGroupError = 'Add at least one person.';
      return;
    }
    loadFriendGroupsIntoState();
    const current = friendGroups.find((group) => group.id === friendGroupEditingId);
    if (!current) {
      friendGroupError = 'Group no longer exists.';
      return;
    }
    const next = friendGroups.map((group) =>
      group.id === friendGroupEditingId
        ? {
            ...group,
            name: groupName,
            members,
            updatedAt: Date.now()
          }
        : group
    );
    saveFriendGroups(next);
    loadFriendGroupsIntoState();
    friendGroupError = null;
    emitGroupsChange();
  };

  const deleteFriendGroupDraft = () => {
    if (!friendGroupEditingId) return;
    loadFriendGroupsIntoState();
    const next = friendGroups.filter((group) => group.id !== friendGroupEditingId);
    saveFriendGroups(next);
    loadFriendGroupsIntoState();
    resetFriendGroupDraft();
    emitGroupsChange();
  };

  const closeModal = () => {
    open = false;
    dispatch('close');
  };

  $: {
    if (open && !wasOpen) {
      openFriendGroupEditor(initialGroupId || '');
    }
    if (!open && wasOpen) {
      friendGroupError = null;
    }
    wasOpen = open;
  }
</script>

{#if open}
  <div class="modal-scrim">
    <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[82vh]">
      <h3 class="text-lg font-semibold modal-title">Friend groups</h3>
      <p class="text-xs text-surface-300 modal-subtitle">Create, edit, update, or delete reusable groups of people.</p>
      <div class="space-y-2 rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 ui-panel">
        <div class="flex items-center justify-between gap-2">
          <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Saved groups</p>
          <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={resetFriendGroupDraft}>
            New draft
          </button>
        </div>
        {#if friendGroups.length > 0}
          <div class="space-y-2">
            {#each friendGroups as group}
              <button
                class={`w-full rounded-xl border px-3 py-2 text-left modal-list-row ${
                  friendGroupEditingId === group.id
                    ? 'border-cyan-300/60 bg-cyan-500/15'
                    : 'border-surface-700'
                }`}
                type="button"
                on:click={() => openFriendGroupEditor(group.id)}
              >
                <p class="text-sm font-semibold text-white">{group.name}</p>
                <p class="text-xs text-surface-300">{group.members.length} member{group.members.length === 1 ? '' : 's'}</p>
              </button>
            {/each}
          </div>
        {:else}
          <p class="text-xs text-surface-300">No groups saved yet.</p>
        {/if}
      </div>

      <label class="block modal-field">
        <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Group name</span>
        <input class="input w-full" bind:value={friendGroupDraftName} placeholder="Group name" />
      </label>

      <div class="space-y-2 rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 ui-panel">
        <div class="flex items-center justify-between gap-2">
          <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Members</p>
          <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={addFriendGroupDraftMember}>
            Add member
          </button>
        </div>
        {#each friendGroupDraftMembers as member, index}
          <div class="grid grid-cols-[1fr_1fr_auto] gap-2 items-center rounded-xl border border-surface-700/80 bg-surface-950/45 p-2 modal-list-row">
            <input
              class="input w-full"
              value={member.name}
              placeholder="Name"
              on:input={(event) => {
                friendGroupDraftMembers = friendGroupDraftMembers.map((row, rowIndex) =>
                  rowIndex === index ? { ...row, name: (event.target as HTMLInputElement).value } : row
                );
              }}
            />
            <input
              class="input w-full"
              value={member.venmoUsername}
              placeholder="Venmo (optional)"
              on:input={(event) => {
                friendGroupDraftMembers = friendGroupDraftMembers.map((row, rowIndex) =>
                  rowIndex === index
                    ? { ...row, venmoUsername: normalizeVenmoUsername((event.target as HTMLInputElement).value) }
                    : row
                );
              }}
            />
            <button
              class="action-btn action-btn-danger action-btn-compact"
              type="button"
              on:click={() => removeFriendGroupDraftMember(index)}
            >
              Remove
            </button>
          </div>
        {/each}
      </div>

      {#if friendGroupError}
        <p class="rounded-lg border border-error-400/40 bg-error-500/10 px-3 py-2 text-xs text-error-100">{friendGroupError}</p>
      {/if}

      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <button class="btn btn-outline w-full" type="button" on:click={saveFriendGroupDraftAsNew}>
          Save as new
        </button>
        <button class="btn btn-outline w-full" type="button" on:click={updateFriendGroupDraft} disabled={!friendGroupEditingId}>
          Update group
        </button>
        <button class="btn btn-outline w-full" type="button" on:click={deleteFriendGroupDraft} disabled={!friendGroupEditingId}>
          Delete group
        </button>
      </div>
      <button class="btn btn-primary w-full" type="button" on:click={closeModal}>
        Done
      </button>
    </div>
  </div>
{/if}
