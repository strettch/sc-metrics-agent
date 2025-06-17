#!/bin/sh
echo "Stopping and disabling sc-agent..."
systemctl stop sc-agent.service
systemctl disable sc-agent.service
