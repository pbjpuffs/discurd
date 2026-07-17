import { create } from 'zustand'

// Mobile UI chrome: which slide-in drawer (if any) is open. On desktop the
// guild rail / channel sidebar / member list are always-visible columns and
// these flags are simply ignored by the layout (see the responsive CSS).
export const useUiStore = create((set) => ({
  navOpen: false, // left drawer: guild rail + channel sidebar
  membersOpen: false, // right drawer: member list

  openNav: () => set({ navOpen: true, membersOpen: false }),
  closeNav: () => set({ navOpen: false }),
  toggleNav: () => set((s) => ({ navOpen: !s.navOpen, membersOpen: false })),

  openMembers: () => set({ membersOpen: true, navOpen: false }),
  closeMembers: () => set({ membersOpen: false }),
  toggleMembers: () => set((s) => ({ membersOpen: !s.membersOpen, navOpen: false })),

  closeDrawers: () => set({ navOpen: false, membersOpen: false }),
}))
