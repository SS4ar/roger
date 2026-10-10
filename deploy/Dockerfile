FROM php:8.3-apache

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        procps \
        net-tools \
        dnsutils \
        iputils-ping \
        curl \
        nano \
        less \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/run/sshd \
    && echo 'root:pwn' | chpasswd \
    && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config \
    && sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
    && rm -f /var/www/html/index.html

RUN printf '%s\n' \
        '<!doctype html>' \
        '<html><head><title>Acme CDN Origin</title></head>' \
        '<body><h1>Acme CDN</h1><p>Static asset origin is running.</p></body></html>' \
    > /var/www/html/index.html

EXPOSE 22 80

CMD ["sh", "-c", "ssh-keygen -A && /usr/sbin/sshd && exec apache2-foreground"]
