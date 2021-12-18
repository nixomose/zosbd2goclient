/* This module handles the (at the moment) single threaded shuttling of data between the block device
 via it's handle_id to the Disk thing it is passed. */

 package zosbd2cmdlib

 import "zosbd2cmdlib/zosbd2share"

type  Block_device_handler struct {

 m_log *Zosbd2cmd_logger;
m_kmod *Kmod
   m_storage * Storage_mechanism
 m_device_name string 
 m_block_device_handle_id uint32
 }



func (b *Block_device_handler) Run_block_for_request( fd int, op *Zosbd2_operation )      {
  xxxz
        /* This is the center of the waiting universe, if this fails, we just keep trying until we can prove
         it'll never succeed, like the device is gone.
         It has to work, which is why it doesn't return anything.
         Well, okay it will only return if we need to exit, which for now is never, until we have
         a good list of things that are permanent failures. */
        while (true)
          {
            Ret r = m_kmod.block_for_request(fd, m_block_device_handle_id, op);
            /* Ahhh, what to do if this fails, wait and try again... */
            /* So it turns out there are cases when we do want to bail, there are some permanent failures, like:
             * block device doesn't exist. For example, somebody sends the kernel module a request to end this block device.
             * the kernel module will queue an exit request to userspace, but if userspace takes too long to come back, it will
             * cancel that request and destroy the block device. the userspace thing never gets the exit request so it goes
             * to respond to its last request, and fails because the block device is no longer there.
             * Then it comes here and spins forever trying to contact the block device which will never return.
             * In that case, we should exit. So we check for the actual error being ENOENT  to fail. We should
             * probably default to failing except in cases where we know something is retryable, but this will do for now. */
            if (r == false)
              {
                if ((r.get_error_code() == ENOENT) || (r.get_error_code() == -ENOENT))
                  {
                    m_log.err("got ENOENT on block device trying to block for request, failing out. error: %s", r.get_error_msg().c_str());
                    return r;
                  }
                m_log.err("Error from block_for_request, sleeping 1 second, error: %s", r.get_error_msg().c_str());
                sleep(1);
              }
            else
              {
                break;
              }
          }
        return Ret();
      }

    Ret block_device_handler::run(void)
      {
        /* So this is the main loop of the block device handler. We initially block waiting for a request and then handle them
         as they come in, and so on, until we get an 'exit' request from the kernel. */

        /* first thing we do is open our shiney new block device */
        string full_device_name = kmod::TXT_DEVICE_PATH + m_device_name;
        int fd;
        Ret ret = m_kmod.open_bd(full_device_name, fd);
        if (ret == false)
          return Ret(ret.get_error_code(), "Error opening block device: %s, handle id: %d, error: %s", full_device_name.c_str(), m_block_device_handle_id,
                     ret.get_error_msg().c_str());

        // the size of the buffer we're going to pass to the kernel. the structure plus the userspacedatabuffer at the end.
        u32 alloc_size = sizeof(zosbd2_operation) + MAX_BIO_SEGMENTS_BUFFER_SIZE_PER_REQUEST; // this is currently hardcoded as the block size 4k * the max number of blocks we can ask for it one list of segment buffers

        void *buffer = malloc(alloc_size);
        if (buffer == NULL)
          return Ret(-ENOMEM, "Unable to allocate memory for zosbd2_operation: %d", ENOMEM);

        zosbd2_operation &op = *((zosbd2_operation*)buffer);

        // get our first request from the kernel
        ret = run_block_for_request(fd, op);
        if (ret == false) // this will never happen, but if it does, it is a permanent failure
          return Ret(EINVAL, "permanent failure from block_for_request, bailing out. error: %s", ret.get_error_msg().c_str());

        // we got our first request, loop forever handling all the requests.
        while (true)
          {
            // switch on the type of operation, all functions block until a new request comes in from the kernel.
            /* The kernel module never returns us an error if something on its side failed, it returns the block layer
             the error. To us, it only calls with requests, so if we get a failure for a request, then it's just something we
             have to retry forever. If the block device disappears somehow, then that will be a permanent failure and we should
             check for those kinds of things and exit. */
            ret = Ret();
            switch (op.header.operation)
              {
              case DEVICE_OPERATION_KERNEL_BLOCK_FOR_REQUEST:
                {
                  ret = run_block_for_request(fd, op);
                  break;
                }
              case DEVICE_OPERATION_KERNEL_READ_REQUEST:
                {
                  ret = m_kmod.respond_to_read_request(fd, m_block_device_handle_id, op, m_storage);
                  break;
                }
              case DEVICE_OPERATION_KERNEL_WRITE_REQUEST:
                {
                  ret = m_kmod.respond_to_write_request(fd, m_block_device_handle_id, op, m_storage);
                  break;
                }
              case DEVICE_OPERATION_KERNEL_DISCARD_REQUEST:
                {
                  ret = m_kmod.respond_to_discard_request(fd, m_block_device_handle_id, op, m_storage);
                  break;
                }
              case DEVICE_OPERATION_KERNEL_USERSPACE_EXIT:
                {
                  m_log.info("got request from kernel to exit. exiting cleanly.");
                  return Ret();
                }
              default:
                {
                  m_log.err("Unsupported request from kernel module: %d", op.header.operation);
                  ret = Ret(-EINVAL, "invalid request, blocking for another request.");
                  break;
                }
              }

            /* check to see if we got an error on the block_for_request part, if so, as above, sleep and try again.
             later version will deal with different kinds of errors more appropriately, exiting or retrying.
             for example if the block device disappears, it's never going to come back, and asking it for things
             will be forever pointless. */

            if (ret == false)
              {
                m_log.err("error from block_for_request call, sleeping and blocking for another request: %s", ret.get_error_msg().c_str());
                ret = run_block_for_request(fd, op);
                if (ret == false)
                  {
                    // this will never happen, but if it does, it is a permanent failure
                    return Ret(-EINVAL, "permanent failure from block_for_request, bailing out. error: %s", ret.get_error_msg().c_str());
                  }
              }
          } // loop forever
      }

