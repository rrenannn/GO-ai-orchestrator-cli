# forge

Orquestrador de agentes de IA escrito em Go. Claude Code atua como **arquiteto**
e **revisor**; Codex atua como **implementador**. O `forge` dirige o ciclo,
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

O `forge` é um binário só. Compile e instale uma vez:

```sh
cd /caminho/deste/repositorio
make install
```

`make install` coloca o binário em `$(go env GOPATH)/bin` — normalmente
`~/go/bin`. Confira que esse diretório está no seu `PATH`:

```sh
which forge      # deve responder /Users/voce/go/bin/forge
forge version
```

Se `which forge` não responder nada, o diretório não está no `PATH`. Adicione
ao seu `~/.zshrc` e abra um terminal novo:

```sh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
```

Em outra máquina que já tenha Go, dá para instalar sem clonar nada:

```sh
go install github.com/rrenannn/GO-ai-orchestrator-cli/cmd/forge@latest
```

Preferindo não instalar, dá para rodar direto do repositório com `make build`
e depois `./bin/forge` — mas aí só funciona de dentro dele.

## Uso

```sh
cd /caminho/do/projeto-que-voce-quer-mexer
forge
```

Só isso. A sessão abre, prepara o projeto se for a primeira vez, e espera você
descrever o que quer construir — como um CLI de chat, mas do outro lado estão
dois agentes se revezando.

```text
› adicionar rate limiting por tenant
```

Enter e o forge assume: Claude planeja, Codex implementa e valida, Claude
revisa, Codex corrige o que a revisão apontou, Claude escolhe a próxima tarefa.
Quando o ciclo termina, o prompt volta para o próximo pedido.

`esc` interrompe a execução sem fechar a sessão; `ctrl+c` no prompt sai.

Para roteiro, CI ou script, os comandos diretos continuam:

```sh
forge init /projeto                          # só prepara os arquivos
forge start /projeto "Adicionar rate limit"  # um pedido, direto
forge status /projeto                        # em que fase está
forge cycle /projeto                         # retoma de onde parou
```

O diretório do projeto é opcional em todos: sem ele, vale o diretório atual.

## A interface

```text
╭─ forge  ~/projetos/api                                    2m14s ─╮
│ ✓ plan → ✓ build → ● review → ○ approved → ○ done      ↺ fixing    │
╰────────────────────────────────────────────────────────────────────╯
╭ PEDIDO                   ╮╭ Live · Plan · Review                   ╮
│ adicionar rate limiting  ││ ── builder (codex) · fase fixing ──    │
│                          ││ ▏editando internal/http/limiter.go     │
│ TAREFAS                  ││ ▏go test ./... ok                      │
│ ✓ T1 add config loader   ││ → reviewing → fixing                   │
│▸◐ T2 add rate limiter    ││                                        │
│ ○ T3 document the API    ││                                        │
│                          ││                                        │
│ EXECUÇÃO                 ││                                        │
│ passos     5/12          ││                                        │
│ correções  1/2           ││                                        │
╰──────────────────────────╯╰────────────────────────────────────────╯
╭────────────────────────────────────────────────────────────────────╮
│ os agentes estão trabalhando · esc interrompe                      │
╰────────────────────────────────────────────────────────────────────╯
 RODANDO  ⣾  BUILDER via codex · 12s
 esc interromper · p pausar · f seguir · tab painéis · r recarregar
```

No prompt (nada rodando):

| Tecla | Ação |
| --- | --- |
| `enter` | envia o pedido e começa a orquestração |
| `↑` `↓` | histórico dos pedidos já enviados |
| `/continue` | retoma o ciclo já registrado no projeto, sem novo pedido |
| `/help` `/quit` | ajuda e saída |
| `esc` | limpa o que foi digitado |
| `ctrl+c` `ctrl+d` | sai |

Durante a execução:

| Tecla | Ação |
| --- | --- |
| `esc` · `ctrl+c` | interrompe a execução; a sessão continua para o próximo pedido |
| `tab` / `1` `2` `3` | alterna **Live** (transcrição), **Plan** (`.agent/PLAN.md`) e **Review** (`.agent/REVIEW.md`) |
| `p` | pausa ou retoma: o laço para **antes** do próximo despacho, sem matar o agente em curso |
| `f` · `g` · `G` | seguir a saída · ir ao topo · ir ao fim |
| `↑` `↓` `PgUp` `PgDn` | rola o painel |
| `r` | recarrega o arquivo do painel |

A interface só aparece quando a saída é um terminal. Em pipe, redirecionamento
ou CI, o `forge` cai automaticamente na transcrição em texto — `--plain`
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
| `FORGE_CLAUDE_CMD` | `claude` | executável do Claude Code |
| `FORGE_CODEX_CMD` | `codex` | executável do Codex |
| `FORGE_BACKGROUND` | detectado | `dark` ou `light`; evita perguntar a cor de fundo ao terminal |

## Máquina de estados

O `forge` é a autoridade sobre o fluxo: depois de cada despacho ele relê
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
| `.agent/REQUEST.md` | forge | o pedido que você digitou |
| `.agent/PLAN.md` | arquiteto | abordagem, riscos e ordem das tarefas |
| `.agent/TASKS.json` | arquiteto | tarefas atômicas com critérios e validação |
| `.agent/REVIEW.md` | revisor | achados e veredito (`APPROVED` / `CHANGES REQUESTED`) |
| `.agent/STATUS.md` | todos | fase atual e tarefa atual |
| `.agent/runs/*.log` | forge | transcrição completa de cada execução |

`CLAUDE.md` e `AGENTS.md` levam as regras de cada papel. O `init` preserva
arquivos existentes; use `--force` para sobrescrever os gerenciados pelo kit.

## Arquitetura

Clean architecture, com a regra de dependência apontando sempre para dentro:

```text
cmd/forge               composition root: único lugar que conhece tudo
internal/cli              delivery: flags, transcrição em texto
internal/tui              delivery: sessão interativa (Bubble Tea)
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

O caso de uso não sabe desenhar nada nem ler teclado: ele publica eventos na
porta `Observer` e consulta a porta `Gate` antes de cada despacho. A sessão
interativa é quem decide *quando* chamar cada caso de uso — o que você digita
vira `Start`, `/continue` vira `Cycle`, e `esc` cancela o contexto daquela
execução sem derrubar a sessão. A transcrição em texto e a TUI
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
