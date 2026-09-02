package cli

const usageText = `forge - orquestrador de agentes de IA
Claude planeja e revisa, Codex implementa.

Uso:
  forge                            abre a sessão interativa no diretório atual
  forge run    [flags] [projeto]   abre a sessão interativa em um projeto
  forge init   [--force] [projeto]
  forge start  [flags] [projeto] <requisito>
  forge cycle  [flags] [projeto]
  forge status [projeto]
  forge version

Na sessão interativa você digita o que quer construir e o forge orquestra:
Claude planeja, Codex implementa, Claude revisa, Codex corrige.

  enter          envia o pedido
  ↑ ↓            histórico de pedidos
  /continue      retoma o ciclo já registrado no projeto
  /help /quit    ajuda e saída
  esc            interrompe a execução em andamento (a sessão continua)
  ctrl+c         interrompe; sem execução em andamento, sai

Flags (run, start, cycle):
  --plain               transcrição em texto, sem interface interativa
  --dry-run             mostra qual agente rodaria, sem despachar nada
  --max-fixes <n>       rodadas de correção por tarefa (padrão 2)
  --max-steps <n>       despachos de agente por execução (padrão 12)
  --timeout <duração>   timeout por agente, ex.: 20m (padrão: sem limite)

Teclas dos painéis (durante a execução):
  tab / 1 2 3    alterna Live, Plan e Review
  p              pausa ou retoma antes do próximo despacho
  f · g · G      seguir, topo, fim
  r              recarrega o arquivo do painel

Ambiente:
  FORGE_CLAUDE_CMD   executável do Claude Code (padrão: claude)
  FORGE_CODEX_CMD    executável do Codex (padrão: codex)

Fluxo:
  request -> arquiteto planeja -> builder implementa e valida
          -> revisor aprova ou rejeita -> builder corrige -> próxima tarefa
`
