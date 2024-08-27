FROM ubuntu:latest

# Install certificates to handle HTTPS requests and clean up package lists to reduce image size
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Set the working directory inside the container
WORKDIR /root/

# Copy the Go binary from the builder stage
COPY GoPool .

# Expose the port the application will run on
EXPOSE 8080

# Run the Go binary
CMD ["./GoPool"]
