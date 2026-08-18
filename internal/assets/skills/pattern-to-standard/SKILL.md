---
name: pattern-to-standard
description: "Trigger: otra vez lo mismo, ya nos paso, esto se repite, deberiamos documentarlo, nuevo estandar. Convierte un aprendizaje repetido en regla aplicable."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Qué hace y qué NO hace

Convierte un aprendizaje suelto en una regla que los trabajos futuros heredan.

**No hay aprendizaje automático.** Una skill es texto estático: no observa ni se actualiza sola. Lo
que cierra el ciclo es que ESTA skill obliga a ESCRIBIR la regla nueva en los tres lugares donde se
lee, para que la próxima sesión ya la traiga. El aprendizaje lo hace el equipo; esto es el
procedimiento para no perderlo.

Para redactar la skill, usar `skill-creator`. Para auditar una existente, `skill-improver`. Esta
cubre el paso anterior: decidir si algo merece volverse estándar y dónde ponerlo.

## Cuándo dispararla

- Un defecto reaparece por segunda vez, aunque sea en otro archivo o repositorio.
- Se pierde tiempo con algo que ya se había resuelto antes y nadie recordaba.
- Se descubre una trampa de una herramienta o un contrato externo que no es evidente al leer.
- Alguien pregunta "¿por qué está hecho así?" y la respuesta sólo vive en la cabeza de uno.

## Filtro: ¿es un patrón o un caso suelto?

RECHAZA promover a estándar si:

- Pasó UNA vez y no hay razón para creer que se repita.
- Es una preferencia de estilo sin consecuencia demostrable.
- La regla no se puede verificar. "Escribir código limpio" no es una regla; "el total se cuenta
  después de filtrar" sí.

REQUIERE, para promoverlo:

- Un síntoma concreto y observable.
- La causa, no sólo el arreglo.
- Una forma de comprobar que se cumple.

Si el defecto costó una tarde y la regla se explica en dos líneas, ya vale la pena.

## Dónde va cada cosa

| Naturaleza | Destino | Formato |
|---|---|---|
| Cómo HACER el trabajo | skill de la capa (`backend-crud-standard`, `frontend-crud-standard`, `db-change-standard`) | prosa breve con el porqué |
| Cómo REVISARLO | `AGENTS.md` del proyecto | `REJECT if` / `REQUIRE` / `PREFER` |
| Contexto para RECORDARLO | engram | `mem_save`, `scope: personal` si cruza proyectos |

Los tres, no uno. La skill enseña, el `AGENTS.md` bloquea, la memoria explica por qué.

## Cómo se escribe la regla

Que se pueda verificar y que diga el porqué. La causa es lo que evita que alguien la "optimice"
de vuelta al defecto.

```text
❌ Cuidar el manejo de fechas.

✅ Marcar DateTimeKind.Utc al leer de la base.
   `datetime` de SQL Server no guarda zona: EF materializa Unspecified, el JSON sale sin la 'Z' y
   el navegador corre toda fecha el tamaño del huso.
   Se comprueba: la misma propiedad debe salir con 'Z' al crear y al leer.
```

Incluir el síntoma. Quien se topa con el problema busca por el síntoma, no por la causa —que
todavía no conoce—.

## Procedimiento

1. Nombrar el patrón en una frase que empiece por el síntoma.
2. Aplicar el filtro de arriba. Si no pasa, guardarlo sólo en memoria y seguir.
3. Escribir la regla en la skill de la capa que corresponda.
4. Agregar la línea `REJECT`/`REQUIRE` al `AGENTS.md` del proyecto.
5. Guardar el contexto en engram con el síntoma, la causa y la evidencia.
6. **Buscar las otras apariciones** del mismo patrón antes de cerrar. Es lo que más se olvida y lo
   que hace que un defecto reaparezca.

## Cuidados

- Las skills se cargan enteras: cada regla agregada le quita atención a las demás. Antes de sumar,
  revisar si alguna sobra.
- Guardian Angel pide 100-200 líneas en el `AGENTS.md`; más largo diluye la revisión.
- Una regla sin causa envejece mal: se cumple sin entenderse hasta que alguien la borra por
  molesta.
