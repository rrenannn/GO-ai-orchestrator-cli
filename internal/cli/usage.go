package cli

const usageText = `maestro - orquestrador de agentes de IA
Claude planeja e revisa, Codex implementa.

Uso:
  maestro init   [--force] [projeto]
  maestro start  [flags] [projeto] <requisito>
  maestro cycle  [flags] [projeto]
  maestro status [projeto]
  maestro version

Flags (start, cycle):
  --plain               transcrição em texto, sem interface interativa
  --dry-run             mostra qual agente rodaria, sem despachar nada
  --max-fixes <n>       rodadas de correção por tarefa (padrão 2)
  --max-steps <n>       despachos de agente por execução (padrão 12)
  --timeout <duração>   timeout por agente, ex.: 20m (padrão: sem limite)

Teclas da interface:
  tab / 1 2 3    alterna Live, Plan e Review
  p              pausa ou retoma antes do próximo despacho
  f · g · G      seguir, topo, fim
  r              recarrega o arquivo do painel
  q              sai (pede confirmação com a execução em andamento)

Ambiente:
  MAESTRO_CLAUDE_CMD   executável do Claude Code (padrão: claude)
  MAESTRO_CODEX_CMD    executável do Codex (padrão: codex)

Fluxo:
  request -> arquiteto planeja -> builder implementa e valida
          -> revisor aprova ou rejeita -> builder corrige -> próxima tarefa
`
