# maestro

Orquestrador de agentes de IA escrito em Go. Claude Code atua como **arquiteto**
e **revisor**; Codex atua como **implementador**. O `maestro` dirige o ciclo,
valida cada transição de estado e para sozinho quando não há mais tarefa aberta
— tudo acompanhado por uma interface de terminal ao vivo.

```text
VOCÊ
  │ request
  ▼
Claude Architect ──plan──▶ Codex Builder ──test──▶ Claude Reviewer
       ▲                        ▲                    │      │
       │                        └────── reject ──────┘   approve
       │                                                    │
       └──────────── próxima tarefa ◀───────────────────────┘
```

## Pré-requisitos

- Go 1.26+ (apenas para compilar)
- `claude` e `codex` instalados e autenticados
- Projeto alvo em Git, com árvore limpa antes de iniciar

## Instalação

```sh
make build      # gera bin/maestro
make install    # instala em $GOPATH/bin
```

## Uso

```sh
# 1. prepara o projeto alvo (instruções dos agentes + artefatos .agent/)
maestro init /caminho/do/projeto

# 2. dispara uma feature: planeja, implementa, valida, revisa e corrige
maestro start /caminho/do/projeto "Adicionar rate limiting por tenant"

# 3. consulta o estado a qualquer momento
maestro status /caminho/do/projeto

# 4. retoma de onde parou (após corrigir algo à mão, por exemplo)
maestro cycle /caminho/do/projeto
```

O diretório do projeto é opcional em todos os comandos: sem ele, vale o
diretório atual.

## A interface

`start` e `cycle` abrem uma interface de tela cheia enquanto os agentes
trabalham:

```text
╭─ maestro  ~/projetos/api                                    2m14s ─╮
│ ✓ plan → ✓ build → ● review → ○ approved → ○ done      ↺ fixing    │
╰────────────────────────────────────────────────────────────────────╯
╭ REQUEST                  ╮╭ Live · Plan · Review                   ╮
│ Adicionar rate limiting  ││ ── builder (codex) · phase fixing ──   │
│                          ││ ▏editando internal/http/limiter.go     │
│ TASKS                    ││ ▏go test ./... ok                      │
│ ✓ T1 add config loader   ││ → reviewing → fixing                   │
│▸◐ T2 add rate limiter    ││                                        │
│ ○ T3 document the API    ││                                        │
│                          ││                                        │
│ RUN                      ││                                        │
│ steps  5/12              ││                                        │
│ fixes  1/2               ││                                        │
╰──────────────────────────╯╰────────────────────────────────────────╯
 RUNNING  ⣾  BUILDER via codex · 12s
 tab panes · p pause · f follow · r reload · q quit
```

| Tecla | Ação |
| --- | --- |
| `tab` / `1` `2` `3` | alterna **Live** (transcrição), **Plan** (`.agent/PLAN.md`) e **Review** (`.agent/REVIEW.md`) |
| `p` | pausa ou retoma: o laço para **antes** do próximo despacho, sem matar o agente em curso |
| `f` · `g` · `G` | seguir a saída · ir ao topo · ir ao fim |
| `↑` `↓` `PgUp` `PgDn` | rola o painel |
| `r` | recarrega o arquivo do painel |
| `q` | sai; com a execução em andamento, pede confirmação e então cancela o agente |

A interface só aparece quando a saída é um terminal. Em pipe, redirecionamento
ou CI, o `maestro` cai automaticamente na transcrição em texto — `--plain`
força esse modo, e `--dry-run` sempre usa ele.

### Flags de `start` e `cycle`

| Flag | Padrão | Efeito |
| --- | --- | --- |
| `--plain` | `false` | transcrição em texto, sem interface interativa |
| `--dry-run` | `false` | mostra qual agente rodaria, sem despachar nada |
| `--max-fixes <n>` | `2` | rodadas de correção permitidas por tarefa |
| `--max-steps <n>` | `12` | despachos de agente permitidos por execução |
| `--timeout <dur>` | sem limite | timeout por agente, ex.: `20m` |

### Variáveis de ambiente

| Variável | Padrão | Efeito |
| --- | --- | --- |
| `MAESTRO_CLAUDE_CMD` | `claude` | executável do Claude Code |
| `MAESTRO_CODEX_CMD` | `codex` | executável do Codex |

## Máquina de estados

O `maestro` é a autoridade sobre o fluxo: depois de cada despacho ele relê
`.agent/STATUS.md` e recusa qualquer transição fora da máquina abaixo — o
builder nunca aprova o próprio trabalho, e um agente que termina sem avançar a
fase interrompe a execução.

```text
planning ─▶ implementing ─▶ reviewing ─┬─▶ approved ─┬─▶ implementing (próxima tarefa)
                 ▲                     │             └─▶ completed
                 └──── fixing ◀────────┘
```

Exceção deliberada: se o arquiteto encerra em `approved` e nenhuma tarefa
continua aberta, o orquestrador conclui a execução e persiste `completed`.

## Contrato entre os agentes

Os agentes se comunicam por arquivos no projeto alvo, e não pelo processo:

| Arquivo | Escrito por | Conteúdo |
| --- | --- | --- |
| `.agent/REQUEST.md` | maestro | requisito da feature |
| `.agent/PLAN.md` | arquiteto | abordagem, riscos e ordem das tarefas |
| `.agent/TASKS.json` | arquiteto | tarefas atômicas com critérios e validação |
| `.agent/REVIEW.md` | revisor | achados e veredito (`APPROVED` / `CHANGES REQUESTED`) |
| `.agent/STATUS.md` | todos | fase atual e tarefa atual |
| `.agent/runs/*.log` | maestro | transcrição completa de cada execução |

`CLAUDE.md` e `AGENTS.md` levam as regras de cada papel. O `init` preserva
arquivos existentes; use `--force` para sobrescrever os gerenciados pelo kit.

## Arquitetura

Clean architecture, com a regra de dependência apontando sempre para dentro:

```text
cmd/maestro               composition root: único lugar que conhece tudo
internal/cli              delivery: flags, transcrição em texto
internal/tui              delivery: interface ao vivo (Bubble Tea)
internal/app/usecase      regras de aplicação: o laço de orquestração
internal/app/event        o que uma execução relata enquanto acontece
internal/app/port         interfaces de saída (implementadas pela infra)
internal/domain           regras de negócio puras, sem I/O
internal/infra            adapters: filesystem, processos, templates, relógio
```

- `internal/domain/workflow` — fases, estados e transições legais
- `internal/domain/task` — tarefa e quadro de tarefas
- `internal/domain/agent` — papéis, CLIs e invocações
- `internal/domain/prompt` — instrução de cada papel (política, não formatação)
- `internal/infra/fsstate` — traduz `.agent/*` em modelo de domínio
- `internal/infra/process` — executa os CLIs e transmite a saída

O caso de uso não sabe desenhar nada: ele publica eventos na porta `Observer` e
consulta a porta `Gate` antes de cada despacho. A transcrição em texto e a TUI
são duas implementações dessas portas — por isso a interface pode mudar por
completo sem tocar em uma linha de regra.

O domínio não importa nada de fora; trocar Codex por outro CLI é escrever um
adapter em `internal/infra/process`.

## Desenvolvimento

```sh
make fmt vet test
make cover
```

Os testes cobrem a máquina de estados, os prompts, o laço de orquestração (com
dublês para as portas), o fluxo de eventos, a pausa, a persistência em
`.agent/`, o CLI ponta a ponta e a renderização da TUI.
