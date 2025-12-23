package alerttasks

import (
	"fmt"
	"log"
	"mosona-manager/internal/config"
	"mosona-manager/internal/email"
	"strings"

	"github.com/nicholas-fedor/shoutrrr"
)

func (a *alertInstance) notifyAll(serverId int64, item, message string) {
	serverName, ok := (*a.serverMap)[serverId]
	if !ok {
		serverName = fmt.Sprintf("Server %d", serverId)
	}

	uri := fmt.Sprintf("%s/%d/monitor", config.DynamicConf.Domain, serverId)
	emailContent, err := email.GetNotificationTemplate(
		serverName,
		item,
		message,
		uri,
	)
	if err != nil {
		log.Println("failed to get email template:", err)
	}

	for _, n := range a.notifications {
		switch n.Module {
		case "email":
			if emailContent != "" {
				if err := email.Send(n.Target, fmt.Sprintf("%s: %s - Alert Notification", serverName, item), emailContent); err != nil {
					log.Println("failed to send alert email:", err)
				}
			}
		case "shoutrrr":
			target := n.Target
			if strings.Contains(target, "telegram://") && !strings.Contains(target, "parsemode=") {
				if strings.Contains(target, "?") {
					target += "&parsemode=HTML"
				} else {
					target += "?parsemode=HTML"
				}
			}
			if err := shoutrrr.Send(target, fmt.Sprintf("<b>%s – %s</b>\n%s\n\n%s", serverName, item, message, uri)); err != nil {
				log.Println("failed to send alert shoutrrr notification:", err)
			}
		}
	}
}
