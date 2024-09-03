# DNS dinámico para DigitalOcean

![DDNS](https://raw.githubusercontent.com/parroquiano/do-dyndns/main/ddns.png)

[README in English](README.md)

`do-dyndns` es un cliente simple de DNS dinámico para DigitalOcean. Se puede usar para
actualizar uno o más registros DNS (para dominios gestionados por DigitalOcean) con la
dirección IP pública de un host cliente.

Por ejemplo, supongamos un dominio `example.com` gestionado por una cuenta de DigitalOcean y un
host `foo` en un laboratorio doméstico. Si dicho host tiene una IP pública externa proporcionada por
un ISP, entonces se puede ejecutar `do-dyndns` periódicamente en `foo` para mantener apuntado un
registro de subdominio `foo.example.com` a la IP pública real.

## Instalación

Descargue el binario apropiado para su plataforma desde [Releases](https://github.com/layfellow/do-dyndns/releases)
y cópielo en cualquier directorio del `PATH` con el nombre `do-dyndns`.

Copie el archivo de configuración de ejemplo `config.json.example` a `$HOME/.config/do-dyndns/config.json`.
Cree primero el directorio `$HOME/.config/do-dyndns`. Como alternativa, puede usar una archivo
`$HOME/.do-dyndnsrc.json` más tradicional.

Edite `config.json` y proporcione los siguientes valores:

- `"token"` (obligatorio): Un [token de acesso personal de DigitalOcean](https://docs.digitalocean.com/reference/api/create-personal-access-token/) . Debe tener permiso *Write*.
- `"log"` (opcional): la ruta completa a un archivo de log.
- `"records"` arreglo (obligatorio): un arreglo de subdominios para actualizar dinámicamente.

`do-dyndns` registra toda su actividad en el archivo `log` si se ejecuta como una tarea cron.
El archivo de `log` no se utiliza si se ejecuta en un shell interactivo. Tampoco se utiliza si se
ejecuta como una tarea programada de systemd, porque stdout se registra automáticamente en este
caso. Si no se proporciona `log`, se usa por defecto `$HOME/.cache/do-dyndns/out.log`.

Para cada elemento de `records`, es necesario proporcionar:

- `"type"`: `"A"`, registro “A” de DNS IPv4, el único tipo soportado por ahora.
- `"subdominio"`: un nombre de subdominio completo para actualizar dinámicamente con la IP pública
actual del host cliente.

## Ejecución como tarea cron o temporizador systemd

Se puede ejecutar `do-dyndns` como una tarea cron. Toda la actividad se registra en el
archivo `log`. (ver archivo de configuración más arriba).

Como alternativa, se puede instalar `do-dyndns` como un temporizador systemd. Tenga en cuenta
que `do-dyndns` no registra su actividad en el archivo `log` cuando se ejecuta como temporizador
systemd. Esto se debe a que el propio systemd se encarga del registro; utilice `journalctl` para consultarlo.

Para más información sobre temporizadores systemd, consulte la [excelente documentación del ArchWiki](https://wiki.archlinux.org/title/Systemd/Timers). (Tenga en cuenta que esta documentación no es específica de Arch Linux; se aplica a cualquier distribución de Linux basada en systemd).

## Plataformas probadas

Probado en Ubuntu 22.10, Fedora 37 y macOS Monterey 12.6.7. Nótese que no hay requisitos especiales
para Linux o macOS, por lo que debería funcionar en cualquier distribución Linux o
versión de macOS razonablemente moderna.

### ¿Y en Windows?

El binario Linux x86-64 *debería* funcionar en Windows 10/11 utilizando [WSL 2](https://learn.microsoft.com/en-us/windows/wsl/about) (Windows Subsystem for Linux).
Desafortunadamente, no tengo acceso a un sistema Windows, así que no puedo confirmarlo.

## Para desarrolladores

`do-dyndns` está escrito en Go 1.19. El código fuente está enteramente contenido en
`main.go`. Pull requests son bienvenidos.

Escribí un pequeño Makefile para ayudarme con las tareas rutinarias.

Para actualizar todas las dependencias de Go, y actualizar `go.mod` y `go.sum`:

    $ make dependencies

Para ejecutar `golangci-lint` localmente (necesita tener instalado [golangci-lint](https://golangci-lint.run/)):

    $ make lint

Para construir el binario para la plataforma de desarrollo:

    $ make build

Para instalar el binario en la ruta por defecto:

    $ make install

Para crear un nuevo Release de GitHub con la última etiqueta (esto requiere el CLI de GitHub):

    $ make releases

---

<a href="https://www.flaticon.com/free-icons/ddns" title="ddns icons">Icono DDNS por Bogdan Rosu - Flaticon</a>
