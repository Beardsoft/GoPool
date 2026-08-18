<script setup lang="ts">
import NimAmount from './ui/NimAmount.vue'
import ExplorerLink from './ui/ExplorerLink.vue'
import { shortAddress, shortHash } from '../utils/format'
import type { EventFact } from '../utils/eventContext'

withDefaults(defineProps<{
  facts: EventFact[]
  layout?: 'compact' | 'fill'
}>(), {
  layout: 'compact',
})
</script>

<template>
  <dl v-if="facts.length" class="event-facts" :data-layout="layout">
    <div v-for="fact in facts" :key="fact.key" class="fact">
      <dt>{{ fact.label }}</dt>
      <dd>
        <NimAmount v-if="fact.kind === 'nim' && fact.luna != null" :luna="fact.luna" />
        <ExplorerLink
          v-else-if="fact.kind === 'address'"
          kind="account"
          copyable
          :value="fact.raw"
          :label="shortAddress(fact.raw)"
          :title="fact.raw"
        />
        <ExplorerLink
          v-else-if="fact.kind === 'tx'"
          kind="transaction"
          copyable
          :value="fact.raw"
          :label="shortHash(fact.raw)"
          :title="fact.raw"
        />
        <span v-else>{{ fact.raw }}</span>
      </dd>
    </div>
  </dl>
</template>

<style scoped>
.event-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 28px;
  margin: 0;
  min-width: 0;
  justify-content: flex-end;
}
.fact {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.fact dt {
  color: var(--app-faint);
  font-size: .7rem;
  font-weight: 800;
  letter-spacing: .02em;
}
.fact dd {
  display: flex;
  align-items: center;
  margin: 0;
  min-width: 0;
  color: var(--app-text);
  font-size: .86rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}
.event-facts[data-layout='fill'] {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(14rem, 1fr));
  gap: 12px 20px;
  justify-content: stretch;
  padding: 14px 16px;
  border-radius: 10px;
  background: var(--surface-2);
}
.event-facts[data-layout='fill'] .fact dd {
  white-space: normal;
}
.event-facts :deep(.explorer-link) {
  min-width: 0;
}
.event-facts :deep(.explorer-link a),
.event-facts :deep(.explorer-mono) {
  overflow: hidden;
  font-size: inherit;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
