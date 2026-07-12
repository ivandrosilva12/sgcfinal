#!/usr/bin/env bash
# Gate de cobertura por camada (Clean Architecture).
# Falha (exit 1) se alguma camada ficar abaixo do limiar. Camadas sem statements
# a cobrir (só placeholders) passam automaticamente.
#
# Limiares (CLAUDE.md §9): domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
# A plataforma (composition root/infra) não é sujeita a gate.
set -euo pipefail

falhou=0

verificar() {
    local nome="$1" alvo="$2" limiar="$3"
    local perfil
    perfil="$(mktemp)"

    if ! go test -covermode=set -coverprofile="$perfil" $alvo >/dev/null 2>&1; then
        echo "  ${nome}: testes FALHARAM"
        falhou=1
        return
    fi

    # Perfil só com a linha 'mode:' → nada a cobrir nesta camada.
    local linhas_dados
    linhas_dados="$(awk '!/^mode:/ { n++ } END { print n + 0 }' "$perfil")"
    if [ "$linhas_dados" -eq 0 ]; then
        echo "  ${nome}: sem statements a cobrir — OK"
        return
    fi

    local total
    total="$(go tool cover -func="$perfil" | awk '/^total:/ {gsub(/%/,"",$NF); print $NF}')"

    if awk -v t="$total" -v l="$limiar" 'BEGIN { exit !(t+0 >= l+0) }'; then
        echo "  ${nome}: ${total}% (limiar ${limiar}%) — OK"
    else
        echo "  ${nome}: ${total}% < ${limiar}% — FALHA"
        falhou=1
    fi
}

echo "Gate de cobertura por camada:"
verificar "domínio"     "./internal/domain/..."      85
verificar "aplicação"   "./internal/application/..." 75
# pgrepo é coberto por testes de integração (sem a tag, aparece a 0%): excluído do gate unitário.
adaptadores_pkgs="$(go list ./internal/adapters/... | grep -v '/pgrepo$' | tr '\n' ' ')"
verificar "adaptadores" "$adaptadores_pkgs" 60

if [ "$falhou" -ne 0 ]; then
    echo "Gate de cobertura FALHOU."
    exit 1
fi
echo "Gate de cobertura OK."
