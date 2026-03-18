<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import {
    loadContacts,
    saveContacts,
    addContact,
    updateContact,
    deleteContact,
    loadRecentPeople,
    promoteRecentToContact,
    normalizeVenmoUsername,
    type Contact,
    type RecentPerson
  } from '$lib/contacts';

  export let open = false;

  const dispatch = createEventDispatcher<{
    close: void;
    contactschange: void;
  }>();

  let contacts: Contact[] = [];
  let recentPeople: RecentPerson[] = [];
  let editingContactId = '';
  let editName = '';
  let editVenmo = '';
  let newName = '';
  let newVenmo = '';
  let error: string | null = null;
  let wasOpen = false;

  const reload = () => {
    contacts = loadContacts();
    recentPeople = loadRecentPeople();
  };

  const emitChange = () => {
    dispatch('contactschange');
  };

  // Recent people not already in contacts
  $: filteredRecent = recentPeople.filter((r) => {
    const key = r.name.trim().toLowerCase();
    return !contacts.some((c) => c.name.trim().toLowerCase() === key);
  });

  const handleAdd = () => {
    const name = newName.trim();
    if (!name) {
      error = 'Name is required.';
      return;
    }
    addContact(name, newVenmo);
    newName = '';
    newVenmo = '';
    error = null;
    reload();
    emitChange();
  };

  const startEdit = (contact: Contact) => {
    editingContactId = contact.id;
    editName = contact.name;
    editVenmo = contact.venmoUsername || '';
    error = null;
  };

  const cancelEdit = () => {
    editingContactId = '';
    editName = '';
    editVenmo = '';
    error = null;
  };

  const saveEdit = () => {
    const name = editName.trim();
    if (!name) {
      error = 'Name is required.';
      return;
    }
    updateContact(editingContactId, { name, venmoUsername: editVenmo });
    cancelEdit();
    reload();
    emitChange();
  };

  const handleDelete = (id: string) => {
    deleteContact(id);
    if (editingContactId === id) cancelEdit();
    reload();
    emitChange();
  };

  const handlePromote = (name: string) => {
    promoteRecentToContact(name);
    reload();
    emitChange();
  };

  const closeModal = () => {
    open = false;
    cancelEdit();
    dispatch('close');
  };

  $: {
    if (open && !wasOpen) {
      reload();
      error = null;
      newName = '';
      newVenmo = '';
    }
    wasOpen = open;
  }
</script>

{#if open}
  <div class="modal-scrim">
    <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[82vh]">
      <h3 class="text-lg font-semibold modal-title">Contacts</h3>
      <p class="text-xs text-surface-300 modal-subtitle">Manage your saved contacts and add from recent bills.</p>

      <!-- Add new contact -->
      <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 space-y-2 ui-panel">
        <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Add contact</p>
        <div class="grid grid-cols-[1fr_1fr_auto] gap-2 items-end">
          <input class="input w-full" placeholder="Name" bind:value={newName} />
          <input class="input w-full" placeholder="Venmo (optional)" bind:value={newVenmo} on:input={() => { newVenmo = normalizeVenmoUsername(newVenmo); }} />
          <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={handleAdd}>Add</button>
        </div>
      </div>

      {#if error}
        <p class="rounded-xl border border-amber-500/40 bg-amber-500/20 px-3 py-2 text-xs text-amber-200">{error}</p>
      {/if}

      <!-- Saved contacts -->
      <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 space-y-2 ui-panel">
        <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Saved contacts ({contacts.length})</p>
        {#if contacts.length > 0}
          <div class="space-y-1 max-h-48 overflow-y-auto">
            {#each contacts as contact}
              {#if editingContactId === contact.id}
                <div class="grid grid-cols-[1fr_1fr_auto_auto] gap-2 items-center rounded-xl border border-cyan-300/60 bg-cyan-500/15 p-2">
                  <input class="input w-full" bind:value={editName} placeholder="Name" />
                  <input class="input w-full" bind:value={editVenmo} placeholder="Venmo (optional)" />
                  <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={saveEdit}>Save</button>
                  <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={cancelEdit}>Cancel</button>
                </div>
              {:else}
                <div class="flex items-center justify-between gap-2 rounded-xl border border-surface-700 px-3 py-2">
                  <div class="min-w-0">
                    <span class="text-sm font-semibold text-white">{contact.name}</span>
                    {#if contact.venmoUsername}
                      <span class="text-xs text-surface-300 ml-2">@{contact.venmoUsername}</span>
                    {/if}
                  </div>
                  <div class="flex gap-1 shrink-0">
                    <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={() => startEdit(contact)}>Edit</button>
                    <button class="action-btn action-btn-danger action-btn-compact" type="button" on:click={() => handleDelete(contact.id)}>Delete</button>
                  </div>
                </div>
              {/if}
            {/each}
          </div>
        {:else}
          <p class="text-xs text-surface-300">No saved contacts yet.</p>
        {/if}
      </div>

      <!-- Recent people -->
      {#if filteredRecent.length > 0}
        <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 space-y-2 ui-panel">
          <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Recent people</p>
          <div class="space-y-1 max-h-36 overflow-y-auto">
            {#each filteredRecent as person}
              <div class="flex items-center justify-between gap-2 rounded-xl border border-surface-700 px-3 py-2">
                <div class="min-w-0">
                  <span class="text-sm text-white">{person.name}</span>
                  {#if person.venmoUsername}
                    <span class="text-xs text-surface-300 ml-2">@{person.venmoUsername}</span>
                  {/if}
                </div>
                <button class="action-btn action-btn-surface action-btn-compact shrink-0" type="button" on:click={() => handlePromote(person.name)}>Save</button>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <button class="btn btn-primary w-full" type="button" on:click={closeModal}>Done</button>
    </div>
  </div>
{/if}
