#!/bin/bash
# Build executable for Linux

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Building Gollama for Linux${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Create bin directory if it doesn't exist
if [ ! -d "bin" ]; then
    echo -e "${YELLOW}Creating bin directory...${NC}"
    mkdir -p bin
fi

# Build gollama server
echo -e "${YELLOW}Building gollama server...${NC}"
if go build -o ./bin/gollama ./cmd/server; then
    echo -e "${GREEN}✓ gollama server built successfully${NC}"
else
    echo -e "${RED}✗ Failed to build gollama server${NC}"
    exit 1
fi

# Build setup tool
echo -e "${YELLOW}Building setup tool...${NC}"
if go build -o ./bin/setup ./cmd/setup; then
    echo -e "${GREEN}✓ Setup tool built successfully${NC}"
else
    echo -e "${RED}✗ Failed to build setup tool${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Build complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Executables created in ./bin/:${NC}"
echo -e "  - gollama (server)"
echo -e "  - setup (setup tool)"
echo ""
echo -e "${YELLOW}To run the server:${NC}"
echo -e "  ./bin/gollama"
echo ""
